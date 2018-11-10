package storage

import (
	"github.com/gobuffalo/packr"
	"fmt"
)

var box = packr.NewBox("./migrations")

func FindMigration(driver string) (string, error) {
	return box.FindString(fmt.Sprintf("migrate_%s.sql", driver))
}