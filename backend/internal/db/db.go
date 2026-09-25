package db

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pinger interface{ Ping(context.Context) error }

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return p, nil
}
func Close(p *pgxpool.Pool) {
	if p != nil {
		p.Close()
	}
}
