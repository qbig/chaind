package pkg

type BackendType string

const (
	EthereumBackendType BackendType = "ETH"
	BitcoinBackendType  BackendType = "BTC"
)

type Backend struct {
	URL    string
	Name   string
	IsMain bool
	Type   BackendType
}
