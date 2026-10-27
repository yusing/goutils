module github.com/yusing/goutils/http/reverseproxy

go 1.27

replace github.com/yusing/goutils => ../..

require (
	github.com/rs/zerolog v1.35.1
	github.com/yusing/goutils v0.9.4
	github.com/yusing/goutils/http v0.9.4
	golang.org/x/net v0.61.0
)

require (
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/yusing/goutils/http => ..
