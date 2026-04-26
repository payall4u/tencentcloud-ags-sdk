// juicefs_mount — 演示如何创建挂载 JuiceFS 卷的 Custom 沙箱，并通过 envd filesystem
// 客户端验证挂载目录可读。
//
// 运行方式：
//
//	source .env.example   # 或 source .env.prod（真实环境）
//	go run main.go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"

	"git.woa.com/ags/ags-go-sdk/connection"
	"git.woa.com/ags/ags-go-sdk/constant"
	"git.woa.com/ags/ags-go-sdk/tool/filesystem"
)

// config 持有从环境变量读取的所有运行时配置。
type config struct {
	// 腾讯云凭证
	SecretID  string
	SecretKey string

	// AGS 服务端点
	Region          string
	Endpoint        string
	DataPlaneDomain string

	// JuiceFS 存储源
	JuicefsBaseURL    string
	JuicefsVolumeName string
	JuicefsToken      string

	// 网络（需与 JuiceFS 元数据引擎同 VPC）
	SubnetID      string
	SecurityGroup string

	// 沙箱配置
	RoleArn    string
	Image      string
	MountPath  string
}

// loadConfig 从环境变量加载配置，缺少必填项时直接退出并列出所有缺失变量。
func loadConfig() config {
	get := func(key string) string { return os.Getenv(key) }

	c := config{
		SecretID:  get("TENCENTCLOUD_SECRET_ID"),
		SecretKey: get("TENCENTCLOUD_SECRET_KEY"),

		Region:          get("AGS_REGION"),
		Endpoint:        get("AGS_ENDPOINT"),
		DataPlaneDomain: get("AGS_DATA_PLANE_DOMAIN"),

		JuicefsBaseURL:    get("JUICEFS_BASE_URL"),
		JuicefsVolumeName: get("JUICEFS_VOLUME_NAME"),
		JuicefsToken:      get("JUICEFS_TOKEN"),

		SubnetID:      get("AGS_SUBNET_ID"),
		SecurityGroup: get("AGS_SECURITY_GROUP_ID"),

		RoleArn:   get("AGS_ROLE_ARN"),
		Image:     get("AGS_IMAGE"),
		MountPath: get("AGS_MOUNT_PATH"),
	}

	required := []struct {
		name, val string
	}{
		{"TENCENTCLOUD_SECRET_ID", c.SecretID},
		{"TENCENTCLOUD_SECRET_KEY", c.SecretKey},
		{"AGS_REGION", c.Region},
		{"AGS_ENDPOINT", c.Endpoint},
		{"AGS_DATA_PLANE_DOMAIN", c.DataPlaneDomain},
		{"JUICEFS_BASE_URL", c.JuicefsBaseURL},
		{"JUICEFS_VOLUME_NAME", c.JuicefsVolumeName},
		{"JUICEFS_TOKEN", c.JuicefsToken},
		{"AGS_SUBNET_ID", c.SubnetID},
		{"AGS_SECURITY_GROUP_ID", c.SecurityGroup},
		{"AGS_ROLE_ARN", c.RoleArn},
		{"AGS_IMAGE", c.Image},
		{"AGS_MOUNT_PATH", c.MountPath},
	}

	var missing []string
	for _, r := range required {
		if r.val == "" {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "缺少以下必填环境变量（请先执行 source .env.example 或 source .env.prod）:")
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", m)
		}
		os.Exit(1)
	}
	return c
}

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// ── 1. 初始化 SDK 客户端 ────────────────────────────────────────────────────
	cred := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = cfg.Endpoint
	client, err := ags.NewClient(cred, cfg.Region, cpf)
	must(err, "NewClient")

	// ── 2. 创建 Custom 沙箱工具（含 JuiceFS StorageMount）──────────────────────
	toolName := fmt.Sprintf("juicefs-example-%d", time.Now().Unix())
	fmt.Printf("→ 创建沙箱工具: %s\n", toolName)

	createReq := ags.NewCreateSandboxToolRequest()
	createReq.ToolName = ptr(toolName)
	createReq.ToolType = ptr("custom")
	createReq.RoleArn = ptr(cfg.RoleArn)
	createReq.Description = ptr("JuiceFS 挂载示例（自动清理）")
	createReq.DefaultTimeout = ptr("15m")
	createReq.NetworkConfiguration = &ags.NetworkConfiguration{
		NetworkMode: ptr("VPC"),
		VpcConfig: &ags.VPCConfig{
			SubnetIds:        []*string{ptr(cfg.SubnetID)},
			SecurityGroupIds: []*string{ptr(cfg.SecurityGroup)},
		},
	}
	createReq.CustomConfiguration = &ags.CustomConfiguration{
		Image:             ptr(cfg.Image),
		ImageRegistryType: ptr("personal"),
		Command:           []*string{ptr("/usr/bin/envd")},
		Args:              []*string{ptr("-port"), ptr("49983")},
		Resources: &ags.ResourceConfiguration{
			CPU:    ptr("2"),
			Memory: ptr("4Gi"),
		},
		Probe: &ags.ProbeConfiguration{
			HttpGet: &ags.HttpGetAction{
				Path:   ptr("/health"),
				Port:   int64Ptr(int64(constant.EnvdPort)),
				Scheme: ptr("HTTP"),
			},
			ReadyTimeoutMs:   int64Ptr(30000),
			ProbeTimeoutMs:   int64Ptr(5000),
			ProbePeriodMs:    int64Ptr(3000),
			SuccessThreshold: int64Ptr(1),
			FailureThreshold: int64Ptr(40),
		},
	}
	createReq.StorageMounts = []*ags.StorageMount{
		{
			Name:      ptr("juicefs-vol"),
			MountPath: ptr(cfg.MountPath),
			ReadOnly:  boolPtr(false),
			StorageSource: &ags.StorageSource{
				Juicefs: &ags.JuicefsStorageSource{
					BaseURL:    ptr(cfg.JuicefsBaseURL),
					VolumeName: ptr(cfg.JuicefsVolumeName),
					Token:      ptr(cfg.JuicefsToken),
				},
			},
		},
	}

	createResp, err := client.CreateSandboxTool(createReq)
	must(err, "CreateSandboxTool")
	toolID := *createResp.Response.ToolId
	fmt.Printf("✅ 工具已创建: id=%s\n", toolID)

	// 程序退出时自动删除工具
	defer func() {
		fmt.Printf("→ 删除工具 %s ...\n", toolID)
		delReq := ags.NewDeleteSandboxToolRequest()
		delReq.ToolId = ptr(toolID)
		if _, err := client.DeleteSandboxTool(delReq); err != nil {
			fmt.Printf("⚠️  DeleteSandboxTool: %v\n", err)
		} else {
			fmt.Printf("✅ 工具已删除\n")
		}
	}()

	// ── 3. 等待工具变为 ACTIVE ──────────────────────────────────────────────────
	fmt.Printf("→ 等待工具 ACTIVE ...\n")
	must(waitForToolActive(ctx, client, toolID, 3*time.Minute), "waitForToolActive")
	fmt.Printf("✅ 工具已就绪\n")

	// ── 4. 启动沙箱实例 ─────────────────────────────────────────────────────────
	fmt.Printf("→ 启动沙箱实例 ...\n")
	startReq := ags.NewStartSandboxInstanceRequest()
	startReq.ToolId = ptr(toolID)
	startReq.Timeout = ptr("10m")
	startResp, err := client.StartSandboxInstance(startReq)
	must(err, "StartSandboxInstance")
	instanceID := *startResp.Response.Instance.InstanceId
	fmt.Printf("✅ 实例已启动: %s\n", instanceID)

	// 程序退出时自动停止实例
	defer func() {
		fmt.Printf("→ 停止实例 %s ...\n", instanceID)
		stopReq := ags.NewStopSandboxInstanceRequest()
		stopReq.InstanceId = ptr(instanceID)
		if _, err := client.StopSandboxInstance(stopReq); err != nil {
			fmt.Printf("⚠️  StopSandboxInstance: %v\n", err)
		} else {
			fmt.Printf("✅ 实例已停止\n")
		}
	}()

	// ── 5. 获取访问 Token ───────────────────────────────────────────────────────
	tokenReq := ags.NewAcquireSandboxInstanceTokenRequest()
	tokenReq.InstanceId = ptr(instanceID)
	tokenResp, err := client.AcquireSandboxInstanceToken(tokenReq)
	must(err, "AcquireSandboxInstanceToken")
	accessToken := *tokenResp.Response.Token
	fmt.Printf("✅ Token 已获取（过期时间: %s）\n", safeStr(tokenResp.Response.ExpiresAt))

	// ── 6. 等待 envd 就绪 ────────────────────────────────────────────────────────
	// 数据面域名格式：{port}-{instanceId}.{region}.{dataPlaneDomain}
	envdDomain := fmt.Sprintf("%d-%s.%s.%s",
		constant.EnvdPort, instanceID, cfg.Region, cfg.DataPlaneDomain)
	fmt.Printf("→ 等待 envd 就绪 (%s) ...\n", envdDomain)
	must(waitForEnvd(ctx, envdDomain, accessToken, 3*time.Minute), "waitForEnvd")
	fmt.Printf("✅ envd 已就绪\n")

	// ── 7. 通过 filesystem 客户端验证 JuiceFS 挂载（带重试）──────────────────────
	fsClient, err := filesystem.New(&connection.Config{
		Domain:      envdDomain,
		AccessToken: accessToken,
	})
	must(err, "filesystem.New")

	fmt.Printf("→ 等待 JuiceFS 挂载（最多 2 分钟）...\n")
	mounted, entries := waitForJuicefsMount(ctx, fsClient, cfg.MountPath, 2*time.Minute)
	if mounted {
		fmt.Printf("✅ JuiceFS 已挂载，%s 下共 %d 个条目:\n", cfg.MountPath, len(entries))
		for _, e := range entries {
			fmt.Printf("   %s\n", e.Name)
		}
	} else {
		fmt.Printf("⚠️  2 分钟内未检测到 JuiceFS 挂载（卷可能为空或挂载失败）\n")
	}

	// ── 8. 从 /proc/mounts 二次确认 ─────────────────────────────────────────────
	verifyProcMounts(ctx, envdDomain, accessToken, cfg.MountPath)
}

// ── 轮询辅助函数 ──────────────────────────────────────────────────────────────

// waitForToolActive 轮询直到工具状态为 ACTIVE 或超时。
func waitForToolActive(ctx context.Context, client *ags.Client, toolID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req := ags.NewDescribeSandboxToolListRequest()
		req.ToolIds = []*string{ptr(toolID)}
		resp, err := client.DescribeSandboxToolList(req)
		if err != nil {
			return fmt.Errorf("DescribeSandboxToolList: %w", err)
		}
		if resp.Response != nil && len(resp.Response.SandboxToolSet) > 0 {
			if s := resp.Response.SandboxToolSet[0].Status; s != nil {
				switch *s {
				case "ACTIVE":
					return nil
				case "FAILED":
					return fmt.Errorf("工具进入 FAILED 状态")
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("等待工具 ACTIVE 超时（%v）", timeout)
}

// waitForEnvd 轮询直到 envd HTTP 端点返回非 5xx 或超时。
func waitForEnvd(ctx context.Context, domain, token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("https://%s/files?path=/&username=root", domain)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-access-token", token)
		if resp, err := httpClient.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("等待 envd 超时（%v）", timeout)
}

// waitForJuicefsMount 每 10 秒轮询一次，直到目录非空或超时。
// 返回 (mounted, entries)。
func waitForJuicefsMount(ctx context.Context, fsClient *filesystem.Client, path string, timeout time.Duration) (bool, []filesystem.EntryInfo) {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, nil
		default:
		}
		attempt++
		entries, err := fsClient.List(ctx, path, &filesystem.ListConfig{User: filesystem.UserRoot})
		if err != nil {
			fmt.Printf("  [%d] List 出错: %v — 重试中 ...\n", attempt, err)
		} else if len(entries) > 0 {
			fmt.Printf("  [%d] ✅ 目录非空（%d 个条目）\n", attempt, len(entries))
			return true, entries
		} else {
			fmt.Printf("  [%d] 目录仍为空，等待中 ...\n", attempt)
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-time.After(10 * time.Second):
		}
	}
	// 超时后最后尝试一次
	entries, err := fsClient.List(ctx, path, &filesystem.ListConfig{User: filesystem.UserRoot})
	if err == nil && len(entries) > 0 {
		return true, entries
	}
	return false, entries
}

// verifyProcMounts 读取沙箱内 /proc/mounts，打印与 mountPath / juicefs 相关的行。
func verifyProcMounts(ctx context.Context, domain, token, mountPath string) {
	url := fmt.Sprintf("https://%s/files?path=/proc/mounts&username=root", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Printf("⚠️  /proc/mounts 请求构建失败: %v\n", err)
		return
	}
	req.Header.Set("x-access-token", token)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("⚠️  /proc/mounts 读取失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	fmt.Println("=== /proc/mounts（仅显示 juicefs/fuse 相关行）===")
	found := false
	for _, line := range splitLines(content) {
		if containsAny(line, mountPath, "juicefs", "fuse") {
			fmt.Printf("  %s\n", line)
			found = true
		}
	}
	if found {
		fmt.Printf("✅ /proc/mounts 中确认 %s 已通过 JuiceFS 挂载\n", mountPath)
	} else {
		fmt.Printf("⚠️  /proc/mounts 中未找到 juicefs 相关条目\n")
	}
}

// ── 通用小工具 ────────────────────────────────────────────────────────────────

func must(err error, label string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal [%s]: %v\n", label, err)
		os.Exit(1)
	}
}

func ptr(s string) *string    { return &s }
func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }
func safeStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
