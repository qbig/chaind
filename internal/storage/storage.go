package storage

import (
	"strings"
	"github.com/pkg/errors"
	"github.com/kyokan/chaind/pkg"
)

type Store interface {
	pkg.Service
	Migrate() error
	GetBackends() ([]pkg.Backend, error)
}

func StorageFromURL(url string) (Store, error) {
	if strings.HasPrefix(url, "file:") {
		return NewSqliteStorage(url)
	}

	return nil, errors.New("unknown database engine")
}