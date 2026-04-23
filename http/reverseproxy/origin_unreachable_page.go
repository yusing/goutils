package reverseproxy

import (
	"bytes"
	_ "embed"
	"io"
	"net/http"

	httputils "github.com/yusing/goutils/http"
)

//go:embed html/origin_unreachable_page.min.html
var originUnreachablePage []byte

func wantsOriginUnreachablePage(req *http.Request) bool {
	if req == nil || req.Method != http.MethodGet {
		return false
	}
	if len(req.Header.Values("Accept")) == 0 {
		return req.URL != nil && req.URL.Path == "/"
	}
	return httputils.GetAccept(req.Header).AcceptHTML()
}

func setNoStoreHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

func newOriginUnreachableResponse(req *http.Request) *http.Response {
	header := make(http.Header)
	body := []byte("Origin server is not reachable.")

	if wantsOriginUnreachablePage(req) {
		body = originUnreachablePage
		header.Set("Content-Type", "text/html; charset=utf-8")
		setNoStoreHeaders(header)
	}

	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "text/plain; charset=utf-8")
	}

	return &http.Response{
		Status:        http.StatusText(http.StatusBadGateway),
		StatusCode:    http.StatusBadGateway,
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		TLS:           req.TLS,
	}
}

// WriteDebugOriginUnreachablePage writes the minified origin-unreachable HTML and no-store
// headers, with status 200 for local preview (e.g. the app debug server).
func WriteDebugOriginUnreachablePage(w http.ResponseWriter) {
	setNoStoreHeaders(w.Header())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(originUnreachablePage)
}
