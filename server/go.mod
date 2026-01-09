module github.com/yusing/goutils/server

go 1.25.5

replace github.com/yusing/goutils => ../

require (
	github.com/pires/go-proxyproto v0.8.1
	github.com/quic-go/quic-go v0.58.0
	github.com/rs/zerolog v1.34.0
	github.com/samber/slog-zerolog/v2 v2.9.0
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.48.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.2.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)
