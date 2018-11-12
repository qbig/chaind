package rpc

import (
	"strings"
	"math/big"
	"github.com/pkg/errors"
)


func De0x(num string) string {
	return strings.Replace(num, "0x", "", 1)
}

func Hex2Big(hex string) (*big.Int, error) {
	val, ok := new(big.Int).SetString(De0x(hex), 16)
	if !ok {
		return nil, errors.New("invalid hex string")
	}

	return val, nil
}

func Hex2Uint64(hex string) (uint64, error) {
	b, err := Hex2Big(hex)
	if err != nil {
		return 0, err
	}

	return b.Uint64(), err
}