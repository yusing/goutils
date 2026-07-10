module github.com/yusing/goutils/http/reverseproxy/integrationtest

go 1.26.4

replace github.com/yusing/goutils/http/reverseproxy => ..

replace github.com/yusing/goutils => ../../..

require (
	github.com/yusing/goutils/http/reverseproxy v0.0.0
	golang.org/x/net v0.57.0
	google.golang.org/grpc v1.80.0
)

require (
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/yusing/goutils v0.7.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
