# goutils/apitypes

Shared JSON shapes for HTTP APIs: an error body, a success body, pagination query
parameters, and a pagination summary, plus helpers to build them. The structs carry
[gin](https://github.com/gin-gonic/gin) binding tags and
[swag](https://github.com/swaggo/swag) annotations, but the package imports neither,
so it adds no dependency.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/apitypes"
```

The package name is `apitypes`. It uses only the standard library and needs Go 1.27 or
newer.

## Quick start

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yusing/goutils/apitypes"
)

func main() {
	show := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Println(string(b))
	}

	show(apitypes.Error("invalid request", errors.New("port must be numeric")))
	show(apitypes.Error("not found"))
	show(apitypes.Success("saved", map[string]any{"id": 7}))
	show(apitypes.QueryResponse{Total: 45, Limit: 20, Offset: 20, HasMore: false})
}
```

Output:

```text
{"message":"invalid request","error":"port must be numeric"}
{"message":"not found"}
{"message":"saved","details":{"id":7}}
{"total":45,"limit":20,"offset":20,"has_more":false}
```

## Responses

| Type | JSON | Builder |
| --- | --- | --- |
| `ErrorResponse` | `{"message": "...", "error": "..."}`; `error` is omitted when empty | `Error(message, err...)` |
| `SuccessResponse` | `{"message": "...", "details": {...}}`; `details` is omitted when empty | `Success(message, extra...)` |

- `Error(message, err...)` uses only the first error. If it has a `Plain() []byte` method,
  such as an [`errs`](../errs/README.md) error, the plain text is used, so the response
  contains no ANSI color codes or Markdown; otherwise it uses `err.Error()`.
  **Passing a `nil` error panics.** Check `err != nil` first or omit the argument.
- `Success(message, extra...)` uses only the first map as `details`.
- Neither builder sets an HTTP status; you pass the status to your framework, for example
  `c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))`.

### `InternalServerError`

```go
func InternalServerError(err error, message string) error
```

Returns an `error` (not a response body) that renders as `message: err`, or just `message`
when `err` is `nil`, and unwraps to `err`, so `errors.Is` and `errors.As` still match. It is
meant to be attached to the framework's error list so a middleware can log the cause and
reply with a generic 500, which keeps internal details out of the response. With gin:

```go
c.Error(apitypes.InternalServerError(err, "failed to read items"))
```

Note the argument order: the error comes first here and the message first in `Error`.

## Pagination

`QueryOptions` binds request query parameters, and `QueryResponse` reports the page:

| Field | Query parameter | Rules (`binding` tag) |
| --- | --- | --- |
| `Limit` | `limit` | required (non-zero), 1 to 20 |
| `Offset` | `offset` | optional, at least 0 |
| `OrderBy` | `order_by` | optional, `created_at` or `updated_at` (`QueryOrder`) |
| `Order` | `order` | optional, `asc` or `desc` (`QueryOrderDirection`) |

Constants: `QueryOrderCreatedAt`, `QueryOrderUpdatedAt`, `QueryOrderDirectionAsc`, and
`QueryOrderDirectionDesc`. `QueryResponse` serializes as
`{"total", "limit", "offset", "has_more"}`.

`QueryOptions` has `form` and `binding` tags only, no `json` tags, so it is meant for
query-string binding. Marshaling it as JSON would use the Go field names (`Limit`, ...).
`Limit` has no default, and a request without `limit` fails validation.

A gin handler using both types (gin is your dependency, not this package's):

```go
func listItems(c *gin.Context) {
	var opts apitypes.QueryOptions
	if err := c.ShouldBindQuery(&opts); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid query", err))
		return
	}
	total := int64(45) // from your store
	c.JSON(http.StatusOK, apitypes.QueryResponse{
		Total:   total,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
		HasMore: int64(opts.Offset+opts.Limit) < total,
	})
}
```

With gin v1.12, `GET /items?limit=10` responds `200` with
`{"total":45,"limit":10,"offset":0,"has_more":true}`, and `GET /items?limit=21` responds
`400` with a body such as:

```json
{"message":"invalid query","error":"Key: 'QueryOptions.Limit' Error:Field validation for 'Limit' failed on the 'max' tag"}
```

## swag annotations

`ErrorResponse` and `SuccessResponse` end with `// @name ...` comments, and their optional
fields have `extensions:"x-nullable"` tags. If you generate OpenAPI documents with swag,
reference the types from handler comments, as the GoDoxy API does:

```go
// @Failure 400 {object} apitypes.ErrorResponse "Bad Request"
```
