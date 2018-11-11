package rpc

const JSONRPC2 = "2.0"
const InternalError = "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32603,\"message\":\"internal error\"}}"

type JSONRPCReq struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Method  string `json:"method"`
	Params  []interface{}
}

type JSONRPCErrorRes struct {
	Jsonrpc string            `json:"jsonrpc"`
	Id      int               `json:"id"`
	Error   *JSONRPCErrorData `json:"error"`
}

type JSONRPCErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JSONRPCRes struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      int         `json:"id"`
	Result  interface{} `json:"result"`
}
