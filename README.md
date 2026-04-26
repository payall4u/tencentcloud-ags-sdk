# tencentcloud-sdk-go/tencentcloud/ags

腾讯云 [AGS（Agent Sandbox）](https://cloud.tencent.com/product/ags) 的 Go SDK 子模块，
基于 [tencentcloud-sdk-go](https://github.com/tencentcloud/tencentcloud-sdk-go) 统一框架实现。

本仓库是 `github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags` 的**独立分发版本**，
在官方 monorepo 合并前可通过 `replace` 指令直接引用。

---

## 快速开始

### 1. 在你的项目中引入

由于本模块尚未合并进官方 monorepo，需在 `go.mod` 中添加 `replace` 指令将其重定向到本仓库：

```bash
# 方式一：使用本地克隆（开发调试推荐）
git clone https://github.com/tencentcloud/tencentcloud-sdk-go-ags.git /path/to/ags-sdk
```

在你的 `go.mod` 中：

```go
require (
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags v0.0.0-00010101000000-000000000000
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.85
)

// 将 ags 子模块重定向到本地克隆路径
replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => /path/to/ags-sdk
```

```bash
# 方式二：直接引用 GitHub（无需本地克隆）
```

```go
require (
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags v0.0.0-00010101000000-000000000000
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.85
)

replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => github.com/tencentcloud/tencentcloud-sdk-go-ags v0.1.0
```

### 2. 初始化客户端

```go
import (
    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
    ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

cred := common.NewCredential(secretID, secretKey)
cpf := profile.NewClientProfile()
cpf.HttpProfile.Endpoint = "ags.tencentcloudapi.com"

client, err := ags.NewClient(cred, "ap-beijing", cpf)
```

### 3. 创建并启动沙箱

```go
// 创建沙箱工具
createReq := ags.NewCreateSandboxToolRequest()
createReq.ToolName = ptr("my-sandbox")
createReq.ToolType = ptr("custom")
createReq.RoleArn  = ptr("qcs::cam::uin/<uin>:roleName/<role>")
// ... 填写 CustomConfiguration、NetworkConfiguration 等

createResp, err := client.CreateSandboxTool(createReq)
toolID := *createResp.Response.ToolId

// 启动实例
startReq := ags.NewStartSandboxInstanceRequest()
startReq.ToolId = ptr(toolID)
startResp, err := client.StartSandboxInstance(startReq)
instanceID := *startResp.Response.Instance.InstanceId

// 获取数据面访问 Token
tokenReq := ags.NewAcquireSandboxInstanceTokenRequest()
tokenReq.InstanceId = ptr(instanceID)
tokenResp, err := client.AcquireSandboxInstanceToken(tokenReq)
token := *tokenResp.Response.Token
```

---

## JuiceFS 存储挂载

本模块在标准 AGS SDK 基础上新增了 `JuicefsStorageSource`，支持将 JuiceFS 卷挂载到沙箱实例中。

```go
createReq.StorageMounts = []*ags.StorageMount{
    {
        Name:      ptr("juicefs-vol"),
        MountPath: ptr("/mnt/juicefs"),
        ReadOnly:  boolPtr(false),
        StorageSource: &ags.StorageSource{
            Juicefs: &ags.JuicefsStorageSource{
                BaseURL:    ptr("http://10.0.0.4:8080"), // 元数据引擎内网地址
                VolumeName: ptr("beijing"),
                Token:      ptr("<juicefs-token>"),
            },
        },
    },
}
```

> **注意**：沙箱实例必须与 JuiceFS 元数据引擎在同一 VPC，需正确配置 `NetworkConfiguration`。

完整可运行示例见 [`examples/juicefs_mount/`](./examples/juicefs_mount/)。

---

## API 一览

| 方法 | 说明 |
|------|------|
| `CreateSandboxTool` | 创建沙箱工具（定义镜像、资源、挂载等） |
| `DeleteSandboxTool` | 删除沙箱工具 |
| `DescribeSandboxToolList` | 查询工具列表及状态 |
| `StartSandboxInstance` | 启动沙箱实例 |
| `StopSandboxInstance` | 停止沙箱实例 |
| `PauseSandboxInstance` | 暂停沙箱实例 |
| `ResumeSandboxInstance` | 恢复沙箱实例 |
| `UpdateSandboxInstance` | 更新实例配置 |
| `DescribeSandboxInstanceList` | 查询实例列表 |
| `AcquireSandboxInstanceToken` | 获取数据面访问 Token |
| `CreateAPIKey` / `DeleteAPIKey` / `DescribeAPIKeyList` | API Key 管理 |
| `CreatePreCacheImageTask` / `DescribePreCacheImageTask` | 镜像预热任务 |

---

## 目录结构

```
.
├── v20250920/          # API 实现（client.go / models.go / errors.go）
├── examples/
│   └── juicefs_mount/  # JuiceFS 挂载完整示例（go run main.go）
└── go.mod              # module github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags
```

---

## 相关链接

- [AGS 产品文档](https://cloud.tencent.com/document/product/1743)
- [tencentcloud-sdk-go 主仓库](https://github.com/tencentcloud/tencentcloud-sdk-go)
- [ags-go-sdk（数据面 SDK）](https://git.woa.com/ags/ags-go-sdk)
