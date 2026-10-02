package httputils

import (
	"net/http"

	"github.com/yusing/goutils/logging"
)

func reqLog(r *http.Request, level logging.Level, message string, fields []logging.Field) {
	attrs := make([]logging.Field, 0, 3+len(fields))
	attrs = append(attrs,
		logging.Field{Key: "remote", Value: r.RemoteAddr},
		logging.Field{Key: "host", Value: r.Host},
		logging.Field{Key: "uri", Value: r.Method + " " + r.RequestURI},
	)
	logging.Log(level, message, append(attrs, fields...)...)
}

func LogError(r *http.Request, message string, fields ...logging.Field) {
	reqLog(r, logging.Error, message, fields)
}

func LogWarn(r *http.Request, message string, fields ...logging.Field) {
	reqLog(r, logging.Warn, message, fields)
}

func LogInfo(r *http.Request, message string, fields ...logging.Field) {
	reqLog(r, logging.Info, message, fields)
}

func LogDebug(r *http.Request, message string, fields ...logging.Field) {
	reqLog(r, logging.Debug, message, fields)
}
