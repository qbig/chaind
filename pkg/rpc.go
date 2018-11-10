package pkg

type EthereumRPCRequest struct {
	Jsonrpc string
	Id      int
	Method  string
	Params  []interface{}
}
