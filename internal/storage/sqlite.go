package storage

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/kyokan/chaind/pkg"
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/log"
)

type SqliteStore struct {
	db *sql.DB
	logger log15.Logger
}

func NewSqliteStorage(url string) (Store, error) {
	db, err := sql.Open("sqlite3", url)
	if err != nil {
		return nil, err
	}

	return &SqliteStore{
		db: db,
		logger: log.NewLog("storage/sqlite"),
	}, nil
}

func (s *SqliteStore) Start() error {
	s.logger.Info("started")
	return nil
}

func (s *SqliteStore) Stop() error {
	return s.db.Close()
}

func (s *SqliteStore) GetBackends() ([]pkg.Backend, error) {
	rows, err := s.db.Query("SELECT url, name, is_main, type FROM backends")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pkg.Backend

	for rows.Next() {
		var backend pkg.Backend
		err := rows.Scan(&backend.URL, &backend.Name, &backend.IsMain, &backend.Type)
		if err != nil {
			return nil, err
		}
		out = append(out, backend)
	}
	return out, rows.Err()
}

func (s *SqliteStore) Migrate() error {
	qStr, err := FindMigration("sqlite")
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(qStr)
	if err != nil {
		return err
	}
	return tx.Commit()
}


