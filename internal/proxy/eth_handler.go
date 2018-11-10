package proxy

import (
	"net/http"
	"encoding/json"
	"errors"
)

type handler func(body []byte, res http.ResponseWriter, req *http.Request) bool

var methodHandlers = map[string]handler{
	"_default": func([]byte, http.ResponseWriter, *http.Request) bool { return true },
}

func HandleETHRequest(body []byte, res http.ResponseWriter, req *http.Request) (bool, error) {
	var data map[string]interface{}
	err := json.Unmarshal(body, &data)
	if err != nil {
		return false, err
	}
	method, ok := data["method"].(string)
	if !ok {
		return false, errors.New("no method found or invalid method type")
	}
	handler, ok := methodHandlers[method]
	if !ok {
		return methodHandlers["_default"](body, res, req), nil
	}

	return handler(body, res, req), nil
}
