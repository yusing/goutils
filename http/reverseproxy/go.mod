module github.com/yusing/goutils/http/reverseproxy

go 1.26.2

replace github.com/yusing/goutils => ../..

require (
	github.com/quic-go/quic-go v0.59.0
	github.com/rs/zerolog v1.35.1
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.53.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.21 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.5.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)
