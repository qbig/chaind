package pkg

import (
	"net/http"
	"bytes"
)

type InterceptedResponse struct {
	body       bytes.Buffer
	statusCode int
	header     http.Header
	res        http.ResponseWriter
	flushed    bool
}

func InterceptResponse(res http.ResponseWriter) *InterceptedResponse {
	return &InterceptedResponse{
		res:    res,
		header: make(http.Header),
	}
}

func (r *InterceptedResponse) Header() http.Header {
	return r.header
}

func (r *InterceptedResponse) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func (r *InterceptedResponse) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *InterceptedResponse) Flush() error {
	r.flushed = true
	for k, vals := range r.header {
		for _, v := range vals {
			r.res.Header().Add(k, v)
		}
	}

	if r.statusCode != 0 {
		r.res.WriteHeader(r.statusCode)
	}

	// copy buffer since other callers might need the body
	buf := bytes.NewBuffer(r.body.Bytes())
	_, err := buf.WriteTo(r.res)
	return err
}

func (r *InterceptedResponse) Body() []byte {
	return r.body.Bytes()
}

func (r *InterceptedResponse) IsOK() bool {
	return r.statusCode == 200 || (r.statusCode == 0 && r.body.Len() > 0)
}
