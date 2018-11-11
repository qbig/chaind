package cache

import (
	"github.com/kyokan/chaind/pkg"
	"time"
)

type Cacher interface {
	pkg.Service
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	SetEx(key string, value []byte, expiration time.Duration) error
	Has(key string) (bool, error)
}