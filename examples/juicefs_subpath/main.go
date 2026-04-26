// juicefs_subpath — 演示如何在启动沙箱实例时通过 MountOptions.SubPath 挂载
// JuiceFS 卷中的子路径（data_01 ~ data_10），每个实例只看到自己的数据分片。
//
// 运行方式：
//
//	source .env.prod   # 或 source .env.example 后填入真实值
//	go run main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"

	"github.com/TencentCloudAgentRuntime/ags-go-sdk/connection"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/constant"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/filesystem"
)

// config 持有从环境变量读取的所有运行时配置。
type config struct {
	SecretID  string
	SecretKey string

	Region          string
	Endpoint        string
	DataPlaneDomain string

	JuicefsBaseURL    string
	JuicefsVolumeName string
	JuicefsToken      string

	SubnetID      string
	SecurityGroup string

	RoleArn   string
	Image     string
	MountPath string
}

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

	required := []struct{ name, val string }{
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
		fmt.Fprintln(os.Stderr, "缺少必填环境变量:")
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", m)
		}
		os.Exit(1)
	}
	return c
}

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// ── 1. 初始化客户端 ────────────────────────────────────────────────────────
	cred := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = cfg.Endpoint
	client, err := ags.NewClient(cred, cfg.Region, cpf)
	must(err, "NewClient")

	// ── 2. 创建共享沙箱工具（JuiceFS 整卷挂载，子路径在启动实例时覆盖）────────
	toolName := fmt.Sprintf("juicefs-subpath-%d", time.Now().Unix())
	fmt.Printf("→ 创建沙箱工具: %s\n", toolName)

	createReq := ags.NewCreateSandboxToolRequest()
	createReq.ToolName = ptr(toolName)
	createReq.ToolType = ptr("custom")
	createReq.RoleArn = ptr(cfg.RoleArn)
	createReq.Description = ptr("JuiceFS SubPath 示例（自动清理）")
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
	// 工具层定义整卷挂载；具体子路径在 StartSandboxInstance 时通过 MountOptions 覆盖
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

	defer func() {
		fmt.Printf("→ 删除工具 %s ...\n", toolID)
		delReq := ags.NewDeleteSandboxToolRequest()
		delReq.ToolId = ptr(toolID)
		if _, err := client.DeleteSandboxTool(delReq); err != nil {
			fmt.Printf("⚠️  DeleteSandboxTool: %v\n", err)
		} else {
			fmt.Println("✅ 工具已删除")
		}
	}()

	// ── 3. 等待工具 ACTIVE ─────────────────────────────────────────────────────
	fmt.Println("→ 等待工具 ACTIVE ...")
	must(waitForToolActive(ctx, client, toolID, 3*time.Minute), "waitForToolActive")
	fmt.Println("✅ 工具已就绪")

	// ── 4. 并发启动 10 个实例，每个实例挂载 data_01 ~ data_10 子路径 ────────────
	const numInstances = 10
	type result struct {
		index      int
		subPath    string
		instanceID string
		entries    []filesystem.EntryInfo
		err        string
	}

	results := make([]result, numInstances)
	var wg sync.WaitGroup

	for i := 0; i < numInstances; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subPath := fmt.Sprintf("data_%02d", idx+1)
			r := result{index: idx, subPath: subPath}

			// 启动实例，通过 MountOptions 覆盖子路径
			startReq := ags.NewStartSandboxInstanceRequest()
			startReq.ToolId = ptr(toolID)
			startReq.Timeout = ptr("10m")
			startReq.MountOptions = []*ags.MountOption{
				{
					Name:    ptr("juicefs-vol"), // 与 StorageMount.Name 对应
					SubPath: ptr(subPath),       // 只暴露该子目录
				},
			}
			startResp, err := client.StartSandboxInstance(startReq)
			if err != nil {
				r.err = fmt.Sprintf("StartSandboxInstance: %v", err)
				results[idx] = r
				return
			}
			instanceID := *startResp.Response.Instance.InstanceId
			r.instanceID = instanceID
			fmt.Printf("  [%s] ✅ 实例已启动: %s\n", subPath, instanceID)

			defer func() {
				stopReq := ags.NewStopSandboxInstanceRequest()
				stopReq.InstanceId = ptr(instanceID)
				client.StopSandboxInstance(stopReq)
				fmt.Printf("  [%s] 实例已停止\n", subPath)
			}()

			// 获取 Token
			tokenReq := ags.NewAcquireSandboxInstanceTokenRequest()
			tokenReq.InstanceId = ptr(instanceID)
			tokenResp, err := client.AcquireSandboxInstanceToken(tokenReq)
			if err != nil {
				r.err = fmt.Sprintf("AcquireSandboxInstanceToken: %v", err)
				results[idx] = r
				return
			}
			accessToken := *tokenResp.Response.Token

			// 等待 envd 就绪
			envdDomain := fmt.Sprintf("%d-%s.%s.%s",
				constant.EnvdPort, instanceID, cfg.Region, cfg.DataPlaneDomain)
			if err := waitForEnvd(ctx, envdDomain, accessToken, 3*time.Minute); err != nil {
				r.err = fmt.Sprintf("waitForEnvd: %v", err)
				results[idx] = r
				return
			}

			// 通过 filesystem 客户端验证挂载内容
			fsClient, err := filesystem.New(&connection.Config{
				Domain:      envdDomain,
				AccessToken: accessToken,
			})
			if err != nil {
				r.err = fmt.Sprintf("filesystem.New: %v", err)
				results[idx] = r
				return
			}

			entries, err := fsClient.List(ctx, cfg.MountPath, &filesystem.ListConfig{
				User: filesystem.UserRoot,
			})
			if err != nil {
				r.err = fmt.Sprintf("List: %v", err)
				results[idx] = r
				return
			}
			r.entries = entries
			results[idx] = r
		}(i)
	}

	wg.Wait()

	// ── 5. 汇总输出 ────────────────────────────────────────────────────────────
	fmt.Println("\n═══════════════════════════ 结果汇总 ═══════════════════════════")
	for _, r := range results {
		if r.err != "" {
			fmt.Printf("  [%s] ❌ %s\n", r.subPath, r.err)
			continue
		}
		if len(r.entries) == 0 {
			fmt.Printf("  [%s] ⚠️  挂载点为空（子路径不存在或 JuiceFS 挂载失败）\n", r.subPath)
			continue
		}
		names := make([]string, 0, len(r.entries))
		for _, e := range r.entries {
			names = append(names, e.Name)
		}
		fmt.Printf("  [%s] ✅ %d 个条目: %v\n", r.subPath, len(r.entries), names)
	}
}

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

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

func must(err error, label string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal [%s]: %v\n", label, err)
		os.Exit(1)
	}
}

func ptr(s string) *string    { return &s }
func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }
