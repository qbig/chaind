package rpc

import "context"

const RequestIDKey = "request_id"

func LogWithRequestID(ctx context.Context, keys ... interface{}) []interface{} {
	return append(keys, []interface{}{
		"request_id",
		ctx.Value(RequestIDKey),
	}...)
}
