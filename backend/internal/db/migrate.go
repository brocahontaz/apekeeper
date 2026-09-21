package db

import (
	"database/sql"
	"embed"
	"errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/httpfs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/http"
)

func Migrate(url string, fs embed.FS) error {
	sqlDB, e := sql.Open("pgx", url)
	if e != nil {
		return e
	}
	defer sqlDB.Close()
	d, e := postgres.WithInstance(sqlDB, &postgres.Config{})
	if e != nil {
		return e
	}
	s, e := httpfs.New(http.FS(fs), ".")
	if e != nil {
		return e
	}
	m, e := migrate.NewWithInstance("httpfs", s, "postgres", d)
	if e != nil {
		return e
	}
	e = m.Up()
	if errors.Is(e, migrate.ErrNoChange) {
		return nil
	}
	return e
}
