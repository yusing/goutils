module github.com/yusing/goutils/server

go 1.26.5

replace github.com/yusing/goutils => ../

require (
	github.com/pires/go-proxyproto v0.15.0
	github.com/quic-go/quic-go v0.61.0
	github.com/rs/zerolog v1.35.1
	github.com/samber/slog-zerolog/v2 v2.9.2
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.58.0
)

require (
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.5.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/samber/slog-common v0.22.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
