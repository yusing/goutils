module github.com/yusing/goutils/http/reverseproxy

go 1.27

replace github.com/yusing/goutils => ../..

require (
	github.com/rs/zerolog v1.35.1
	github.com/yusing/goutils v0.9.3
	github.com/yusing/goutils/http v0.9.3
	golang.org/x/net v0.59.0
)

require (
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/yusing/goutils/http => ..
