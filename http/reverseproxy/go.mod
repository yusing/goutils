module github.com/yusing/goutils/http/reverseproxy

go 1.26.0

replace github.com/yusing/goutils => ../..

require (
	github.com/quic-go/quic-go v0.59.0
	github.com/rs/zerolog v1.34.0
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.50.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.4.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)
