package rpc

const JSONRPC2 = "2.0"
const InternalError = "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32603,\"message\":\"internal error\"}}"

type JSONRPCReq struct {
	Jsonrpc string
	Id      int
	Method  string
	Params  []interface{}
}

type JSONRPCErrorRes struct {
	Jsonrpc string
	Id int
	Error   *JSONRPCErrorData
}

type JSONRPCErrorData struct {
	Code    int
	Message string
}

type JSONRPCRes struct {
	Jsonrpc string
	Id      int
	Result  interface{}
}