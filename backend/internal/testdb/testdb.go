// Package testdb provides isolated PostgreSQL databases for integration tests.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/db"
	"github.com/brocahontaz/apekeeper/backend/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sequence atomic.Uint64

// New creates, migrates, and opens a database dedicated to t. The returned
// pool and database are closed and dropped when t finishes.
func New(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	baseURL := os.Getenv("DATABASE_URL")
	if baseURL == "" {
		t.Skip("DATABASE_URL is unset; start Docker and set it to run database integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	name := fmt.Sprintf("apekeeper_test_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), sequence.Add(1))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanup, err := pgxpool.New(cleanupCtx, baseURL)
		if err != nil {
			t.Errorf("open database cleanup connection: %v", err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.Exec(cleanupCtx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database %s: %v", name, err)
		}
	})

	testURL, err := databaseURL(baseURL, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(testURL, migrations.FS); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(context.Background(), testURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, testURL
}

func databaseURL(baseURL, name string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}
