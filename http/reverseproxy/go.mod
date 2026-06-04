module github.com/yusing/goutils/http/reverseproxy

go 1.26.3

replace github.com/yusing/goutils => ../..

require (
	github.com/quic-go/quic-go v0.59.1
	github.com/rs/zerolog v1.35.1
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.55.0
)

require (
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)
