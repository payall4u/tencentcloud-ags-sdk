# tencentcloud-sdk-go/tencentcloud/ags

腾讯云 AGS（Agent Sandbox）Go SDK，基于 [tencentcloud-sdk-go](https://github.com/tencentcloud/tencentcloud-sdk-go) 框架实现。

## 安装

本模块尚未合并进官方 monorepo，需在项目的 `go.mod` 中用 `replace` 指令引用：

```go
require (
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags v1.3.81
    github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.86
)

// 将 ags 子模块重定向到本仓库
replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => github.com/payall4u/tencentcloud-ags-sdk v0.1.0
```

本地开发时也可指向本地路径：

```go
replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => /path/to/tencentcloud-ags-sdk
```

## 快速开始

```go
import (
    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
    ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

cred := common.NewCredential(secretID, secretKey)
cpf  := profile.NewClientProfile()
cpf.HttpProfile.Endpoint = "ags.tencentcloudapi.com"

client, _ := ags.NewClient(cred, "ap-beijing", cpf)

// 启动沙箱实例
startReq := ags.NewStartSandboxInstanceRequest()
startReq.ToolId = &toolID
resp, _ := client.StartSandboxInstance(startReq)
```

完整示例（含 JuiceFS 存储挂载）见 [`examples/juicefs_mount/`](./examples/juicefs_mount/)。

## 相关链接

- [AGS 产品文档](https://cloud.tencent.com/document/product/1743)
- [tencentcloud-sdk-go 主仓库](https://github.com/tencentcloud/tencentcloud-sdk-go)
- [ags-go-sdk（数据面 SDK）](https://github.com/TencentCloudAgentRuntime/ags-go-sdk)
