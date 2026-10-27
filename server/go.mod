module github.com/yusing/goutils/server

go 1.27

replace github.com/yusing/goutils => ../

require (
	github.com/pires/go-proxyproto v0.16.0
	github.com/quic-go/quic-go v0.64.1
	github.com/rs/zerolog v1.35.1
	github.com/samber/slog-zerolog/v2 v2.9.2
	github.com/yusing/goutils v0.9.4
	github.com/yusing/goutils/http v0.9.4
	golang.org/x/net v0.61.0
)

require (
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.5.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/samber/slog-common v0.22.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/yusing/goutils/http => ../http
