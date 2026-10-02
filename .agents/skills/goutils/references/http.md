# HTTP, proxy, WebSocket, and server modules

| Module (`go get …@<version>`) | Packages | Brings in |
| --- | --- | --- |
| `github.com/yusing/goutils/http` | `httputils` (path `.../http`), `httpheaders`, `accesslog` | x/net |
| `github.com/yusing/goutils/http/reverseproxy` | `reverseproxy` | zerolog, x/net, `http` module |
| `github.com/yusing/goutils/http/websocket` | `websocket` | gin, gorilla/websocket, zerolog |
| `github.com/yusing/goutils/server` | `server` | quic-go, go-proxyproto, zerolog, `http` module |

Every binary that links `httputils`, `reverseproxy`, `websocket`, or `server` must be built and
tested with `-ldflags=-checklinkname=0` (see `SKILL.md`). `httpheaders`, `accesslog`, and `cache`
alone do not need it.

## httputils

- `ReadAllBody(resp)` / `ReadAllRequestBody(req)` return `(b, release, err)`: call `release(b)`
  exactly once and do not use `b` afterwards. On error both are nil. They neither close the body
  nor limit its size; wrap it in `http.MaxBytesReader` or `io.LimitReader` for untrusted input.
- Response rewriting middleware:

  ```go
  func rewrite(next http.Handler) http.Handler {
  	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		rm := httputils.NewResponseModifier(w)
  		defer rm.FlushRelease() // sends the buffered response; required
  		next.ServeHTTP(rm, r)
  		if httputils.GetContentType(rm.Header()).IsJSON() {
  			body := rm.Content()
  			rm.ResetBody()
  			rm.Write(transform(body))
  		}
  	})
  }
  ```

  `NewResponseModifier` buffers the whole response until `FlushRelease`; forgetting it sends an
  empty 200. Do not use `Content()` or `SharedData()` after release. `SetMaxBufferedBytes` makes
  oversized responses switch permanently to passthrough. For streaming-heavy routes use
  `NewLazyResponseModifier(w, shouldBuffer)`, which buffers only when `shouldBuffer(header)` is
  true. `NewModifyResponseWriter` is deprecated.
- `NewInterceptedTransport(rt, intercept)` lets a client transport short-circuit responses: return
  `true, httputils.NewRequestInterceptedError(resp, data)` and recover `data` with
  `AsRequestInterceptedError`.
- `GetSharedData(w)` returns a per-request `Cache` of parsed cookies, queries, basic auth, and
  remote IP when `w` is a `ResponseModifier` (otherwise a fresh one each call). Calling
  `GetBasicAuth` twice on the same cache can panic when the request has no credentials.
- `LogError`/`LogWarn`/`LogInfo`/`LogDebug(r, msg, fields...)` log through `goutils/logging` with
  `remote`, `host`, and `uri` fields.
- `IsUnexpectedError(err)` filters out client disconnects and closed streams before you log
  copy errors.

## httpheaders

- `RemoveHopByHopHeaders` strips hop-by-hop headers including `Upgrade`; `RemoveHop` keeps a
  WebSocket handshake intact. `IsWebsocket`, `UpgradeType`, and `IsGrpcOrSSE` classify requests.
- `AppendCSP(w, r, directives, sources)` reads the starting policy from `r.Header`, not
  `w.Header()`, and writes the result to `w`.
- `FilterHeaders(h, nil)` returns `h` itself, not a copy.

## reverseproxy

```go
target, _ := url.Parse("http://127.0.0.1:8080/base") // or h2c://host:port for cleartext gRPC
rp := reverseproxy.NewReverseProxy("backend", target, http.DefaultTransport)
rp.AccessLogger = myAccessLogger // optional accesslog.AccessLogger
http.Handle("/", rp)
```

- Always use the constructor: a nil transport panics, and `ServeHTTP` dispatches through the
  `HandlerFunc` field it sets (wrap that field to add behavior). There is no `Director`, `Rewrite`,
  or `ErrorHandler`; adjust requests in a handler in front of the proxy and responses with
  `ModifyResponse`.
- Paths join the target base (`/dir` becomes `/base/dir`). The proxy sets `X-Forwarded-For`
  (appending to the incoming chain), `-Method`, `-Proto`, `-Host`, and `-Uri`, and trusts incoming
  `X-Forwarded-For` and `X-Forwarded-Proto: https`. Strip them first when clients are untrusted.
- An unreachable origin gives a synthetic 502 (HTML for browser GETs, text otherwise).
  `AccessLogger.LogError` runs, then `ModifyResponse` and `LogRequest` see the 502.
  `AccessLogger` implementations must be concurrency-safe; the proxy never calls `Close`.
- `OnSchemeMisMatch(current) (retryScheme, retry)` retries once on an HTTP/HTTPS mismatch, only
  for requests without a body.
- Streaming responses (gRPC, SSE, no Content-Length) flush after every write. The proxy logs
  through zerolog's global logger; the embedded `zerolog.Logger` field does not redirect it.

## websocket

Works only with gin (`*gin.Context`). The handler must stay running for the life of the
connection:

```go
func handle(c *gin.Context) {
	m, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		return // the upgrade already wrote the HTTP error
	}
	defer m.Close()
	for {
		select {
		case <-m.Done():
			return
		case msg := <-m.ReadCh():
			_ = m.WriteData(websocket.TextMessage, msg, 5*time.Second)
		}
	}
}
```

- Heartbeat: the client must send a text `ping` every few seconds (the server answers `pong`).
  The server closes the connection about 5-6 s after the last ping, so plain browser clients
  need that loop.
- Keep draining `ReadCh` (it is never closed; select on `Done()`). A stalled reader also stalls
  ping handling and drops the connection.
- After a clean close, `ReadJSON`/`ReadBinary` can return a nil error with no data; check
  `Done()` or `Context().Err()`. Write timeouts surface as a `net.Error` timeout, not
  `ErrWriteTimeout`.
- `NewReader()` returns whole messages and needs a read buffer at least as large as a message;
  prefer `ReadCh`, `ReadJSON`, or `ReadBinary`.
- Use `CopyJSONStream(r)` or `CopyTextLines(r)` to turn a byte stream into one message per JSON
  value or line, `PeriodicWrite(interval, get)` for deduplicated polling updates, and
  `WriteJSON` for single values. JSON uses `encoding/json/v2`.
- The default origin check accepts no `Origin`, a same-host origin, or any origin when Host is
  `localhost`/`127.0.0.1`. Supply a stricter gorilla `*websocket.Upgrader` (import gorilla under
  an alias) via `c.Set("upgrader", u)` before upgrading.

## server

```go
app := task.RootTask("app", true)
_, err := server.StartServer(app, server.Options{
	Name:     "api",
	HTTPAddr: ":8080",
	Handler:  mux, // required; nil panics per request
})
if err != nil {
	log.Fatal(err)
}
task.WaitExit(5)
```

- Each listener runs only when its address is set (`HTTPAddr`, `HTTPSAddr`). HTTPS also needs
  `CertProvider`, and `GetCert(nil)` must succeed, otherwise HTTPS is skipped without an error.
  The plain HTTP listener serves h2c too.
- `StartServer` returns once listening. Shutdown follows the parent task: listeners close, and
  in-flight requests get 1 s and see their request context canceled.
- `(*Server).Start(parent, http3Enabled)` enables HTTP/3 on the HTTPS address (with `Alt-Svc`).
  If HTTPS fails after HTTP started, HTTP keeps serving until the parent is canceled.
- PROXY protocol: build a policy with `server.NewProxyProtocolPolicy(ProxyProtocolConfig{Mode:
  "mixed" or "required", TrustedProxies: [...]})`. Do not use the legacy
  `SupportProxyProtocol`, which trusts any peer. A PROXY policy combined with HTTPS currently
  panics at start, so check `server/README.md` before combining them.
- The generic `server.Start(task, srv, opts...)` serves a single `*http.Server` or HTTP/3 server
  and returns the bound port. On a listen error it does not finish the task, so call
  `t.Finish(err)` yourself. `WithTCPWrappers` after `WithACL` discards the ACL wrapper; pass the
  ACL last.
- Logs through zerolog's global logger.
