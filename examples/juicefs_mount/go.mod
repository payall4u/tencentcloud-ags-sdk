module tencentcloud-ags-sdk/examples/juicefs_mount

go 1.22

require (
	github.com/TencentCloudAgentRuntime/ags-go-sdk v0.1.5
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags v1.3.81
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.86
)

require (
	connectrpc.com/connect v1.18.1 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
)

replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => ../../
