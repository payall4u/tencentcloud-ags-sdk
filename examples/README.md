# AGS SDK Examples

本目录收录可直接运行的示例程序，覆盖 AGS（Agent Sandbox）SDK 的常见使用场景。

## 示例列表

| 目录 | 说明 |
|------|------|
| [`juicefs_mount/`](./juicefs_mount/) | 创建挂载 JuiceFS 卷的 Custom 沙箱，并通过 filesystem 客户端验证挂载 |

---

## juicefs_mount — JuiceFS 存储挂载验证

### 功能概述

演示如何通过 AGS SDK 完整走通以下流程：

1. **CreateSandboxTool** — 创建一个 `custom` 类型沙箱工具，配置 JuiceFS `StorageMount`
2. **等待工具就绪** — 轮询 `DescribeSandboxToolList` 直到状态变为 `ACTIVE`
3. **StartSandboxInstance** — 启动沙箱实例
4. **AcquireSandboxInstanceToken** — 获取数据面访问 Token
5. **等待 envd 就绪** — 轮询 HTTP 健康检查端点
6. **filesystem 客户端验证** — 通过 `ags-go-sdk` 的 `filesystem.Client` 列出挂载目录，带重试逻辑（挂载可能慢半拍）
7. **/proc/mounts 二次确认** — 读取沙箱内核挂载表，确认 JuiceFS fuse 条目
8. **自动清理** — 程序退出时 `StopSandboxInstance` + `DeleteSandboxTool`

### 前置条件

| 条件 | 说明 |
|------|------|
| 腾讯云账号 | 需要有 AGS 服务访问权限 |
| SecretId / SecretKey | 通过环境变量传入，见下方 |
| VPC 网络 | 沙箱实例与 JuiceFS 元数据引擎需在同一 VPC |
| JuiceFS 元数据引擎 | 示例中使用内网地址 `http://10.0.0.4:8080` |
| CAM 角色 | 沙箱需要 `RoleArn` 才能拉取私有镜像和挂载存储 |

### 快速开始

```bash
# 1. 进入示例目录
cd examples/juicefs_mount

# 2. 配置环境变量（二选一）
#    a) 用占位示例（需手动填写真实值）
source .env.example

#    b) 用预填好的本地配置（.env.prod 不纳入版本控制）
source .env.prod

# 3. 运行
go run main.go
```

> **环境变量说明**：`.env.example` 是已提交的模板文件，所有敏感字段均为占位符，
> 可复制为 `.env.prod` 并填入真实值使用。`.env.prod` 已被 `.gitignore` 排除，
> 不会误提交到代码仓库。

### 预期输出

```
→ 创建沙箱工具: juicefs-example-1714122345
✅ 工具已创建: id=sdt-xxxxxxxx
→ 等待工具 ACTIVE ...
✅ 工具已就绪
→ 启动沙箱实例 ...
✅ 实例已启动: si-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
✅ Token 已获取（过期时间: 2026-04-26T12:00:00Z）
→ 等待 envd 就绪 (49983-si-xxx.ap-beijing.tencentags.com) ...
✅ envd 已就绪
→ 等待 JuiceFS 挂载（最多 2 分钟）...
  [1] 目录仍为空，等待中 ...
  [2] ✅ 目录非空（3 个条目）
✅ JuiceFS 已挂载，/mnt/juicefs 下共 3 个条目:
   data
   models
   checkpoints
=== /proc/mounts（仅显示 juicefs/fuse 相关行）===
  JuiceFS:beijing /mnt/juicefs fuse.juicefs rw,relatime,...
✅ /proc/mounts 中确认 /mnt/juicefs 已通过 JuiceFS 挂载
→ 停止实例 si-xxx ...
✅ 实例已停止
→ 删除工具 sdt-xxx ...
✅ 工具已删除
```

### 关键配置说明

#### StorageMount（JuiceFS）

```go
createReq.StorageMounts = []*ags.StorageMount{
    {
        Name:      ptr("juicefs-vol"),   // 挂载卷名称（任意）
        MountPath: ptr("/mnt/juicefs"),  // 沙箱内挂载路径
        ReadOnly:  boolPtr(false),       // JuiceFS 当前不支持 ReadOnly，填 false
        StorageSource: &ags.StorageSource{
            Juicefs: &ags.JuicefsStorageSource{
                BaseURL:    ptr("http://10.0.0.4:8080"), // 元数据引擎地址
                VolumeName: ptr("beijing"),               // 卷名
                Token:      ptr("<juicefs-token>"),       // 访问 Token
            },
        },
    },
}
```

#### CustomConfiguration（envd 镜像必填字段）

`custom` 类型工具的以下三个字段均为**必填**：

```go
createReq.CustomConfiguration = &ags.CustomConfiguration{
    Image:             ptr("ccr.ccs.tencentyun.com/..."),  // 镜像地址
    ImageRegistryType: ptr("personal"),                     // personal 或 enterprise
    Command:           []*string{ptr("/usr/bin/envd")},    // 启动命令（必填）
    Args:              []*string{ptr("-port"), ptr("49983")},
    Resources: &ags.ResourceConfiguration{                  // 资源配置（必填）
        CPU:    ptr("2"),
        Memory: ptr("4Gi"),
    },
    Probe: &ags.ProbeConfiguration{                        // 健康探针（必填）
        HttpGet: &ags.HttpGetAction{
            Path:   ptr("/health"),
            Port:   int64Ptr(49983),
            Scheme: ptr("HTTP"),
        },
        ReadyTimeoutMs:   int64Ptr(30000), // 最大 30000ms
        ProbeTimeoutMs:   int64Ptr(5000),
        ProbePeriodMs:    int64Ptr(3000),
        SuccessThreshold: int64Ptr(1),
        FailureThreshold: int64Ptr(40),
    },
}
```

> **注意**：`ReadyTimeoutMs` 上限为 30000（30s），超过会报参数校验错误。

#### 数据面域名格式

```
{envdPort}-{instanceId}.{region}.tencentags.com
```

例如：`49983-si-abc123.ap-beijing.tencentags.com`

#### filesystem 客户端需使用 root 用户

JuiceFS 挂载点由系统级进程挂载，需以 root 身份访问：

```go
entries, err := fsClient.List(ctx, "/mnt/juicefs", &filesystem.ListConfig{
    User: filesystem.UserRoot, // 必须是 root，否则可能返回空或报权限错误
})
```

### 自定义配置

所有运行时参数均通过环境变量传入，复制 `.env.example` 为 `.env.prod` 并填写真实值即可：

```bash
# 在 examples/juicefs_mount/ 目录下
cp .env.example .env.prod
# 编辑 .env.prod，填入真实的 SecretId / SecretKey / JuiceFS Token 等
source .env.prod
go run main.go
```

| 环境变量 | 说明 | 示例值 |
|----------|------|--------|
| `TENCENTCLOUD_SECRET_ID` | 腾讯云 SecretId | `AKIDxxxxxxxx` |
| `TENCENTCLOUD_SECRET_KEY` | 腾讯云 SecretKey | `xxxxxxxx` |
| `AGS_REGION` | AGS 服务地域 | `ap-beijing` |
| `AGS_ENDPOINT` | AGS API 接入点 | `ags.tencentcloudapi.com` |
| `AGS_DATA_PLANE_DOMAIN` | 数据面域名后缀 | `tencentags.com` |
| `JUICEFS_BASE_URL` | JuiceFS 元数据引擎地址（内网） | `http://10.0.0.4:8080` |
| `JUICEFS_VOLUME_NAME` | JuiceFS 卷名 | `beijing` |
| `JUICEFS_TOKEN` | JuiceFS 访问 Token | `8ac8a5...` |
| `AGS_SUBNET_ID` | VPC 子网 ID（需与 JuiceFS 同 VPC） | `subnet-xxxxxxxx` |
| `AGS_SECURITY_GROUP_ID` | 安全组 ID | `sg-xxxxxxxx` |
| `AGS_ROLE_ARN` | CAM 角色 ARN | `qcs::cam::uin/xxx:roleName/xxx` |
| `AGS_IMAGE` | 含 envd 的容器镜像 | `ccr.ccs.tencentyun.com/ns/repo:tag` |
| `AGS_MOUNT_PATH` | JuiceFS 在沙箱内的挂载路径 | `/mnt/juicefs` |

### 常见问题

| 错误信息 | 原因 | 解决方法 |
|----------|------|----------|
| `CustomConfiguration.Command is required` | 未填写启动命令 | 设置 `Command: []*string{ptr("/usr/bin/envd")}` |
| `CustomConfiguration.Resources is required` | 未填写资源配置 | 设置 `Resources` 字段 |
| `CustomConfiguration.Probe is required` | 未填写健康探针 | 设置 `Probe` 字段 |
| `ReadyTimeoutMs must be at most 30000` | 超时值过大 | 将 `ReadyTimeoutMs` 改为 ≤ 30000 |
| `Sandbox sdt-xxx is not active` | 工具还在 CREATING | 调用 `waitForToolActive` 等待状态变为 ACTIVE 再启动实例 |
| `startup command not found` | 命令路径错误 | 确认镜像内可执行文件的完整路径，如 `/usr/bin/envd` |
| 挂载目录为空（0 条目）| JuiceFS 挂载稍慢 | 使用重试逻辑（示例已内置，最多等待 2 分钟） |
