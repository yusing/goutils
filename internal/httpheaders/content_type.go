package httpheaders

import (
	strutils "github.com/yusing/goutils/strings"
	"mime"
	"net/http"
	"strings"
)

// Source: http/httpheaders/utils.go:121:132@b0525ee3 IsGrpcOrSSE
func IsGrpcOrSSE(h http.Header) bool {
	contentType := h.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			if strings.EqualFold(mediaType, "text/event-stream") || strutils.HasPrefixFold(mediaType, "application/grpc") {
				return true
			}
		}
	}
	return false
}
