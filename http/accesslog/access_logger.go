package accesslog

import "net/http"

type AccessLogger interface {
	LogRequest(req *http.Request, res *http.Response)
	LogError(req *http.Request, err error)
	Close() error
}
