module github.com/yusing/goutils/http/reverseproxy/integrationtest

go 1.27

replace github.com/yusing/goutils/http/reverseproxy => ..

replace github.com/yusing/goutils => ../../..

require (
	github.com/yusing/goutils/http/reverseproxy v0.9.3
	golang.org/x/net v0.61.0
	google.golang.org/grpc v1.84.0
)

require (
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/yusing/goutils v0.9.3 // indirect
	github.com/yusing/goutils/http v0.9.3 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261026165753-2ffab32bce7e // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/yusing/goutils/http => ../..
