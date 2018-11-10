package pkg

import (
	"net/http"
)

type WrappedRequest struct {
	req  *http.Request
	body []byte
}

func WrapRequest(req *http.Request, body []byte) *WrappedRequest {
	return &WrappedRequest{
		req:  req,
		body: body,
	}
}

func (r *WrappedRequest) Method() string {
	return r.req.Method
}

func (r *WrappedRequest) Header() http.Header {
	clone := make(http.Header)
	for k, v := range r.req.Header {
		clone[k] = v
	}
	return clone
}

func (r *WrappedRequest) Body() []byte {
	return r.body
}

func (r *WrappedRequest) RemoteAddr() string {
	return r.req.RemoteAddr
}
