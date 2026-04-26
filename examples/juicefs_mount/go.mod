module tencentcloud-ags-sdk/examples/juicefs_mount

go 1.22

require (
	git.woa.com/ags/ags-go-sdk v0.0.8
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags v0.0.0-00010101000000-000000000000
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.85
)

require (
	connectrpc.com/connect v1.18.1 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
)

replace github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags => ../../
