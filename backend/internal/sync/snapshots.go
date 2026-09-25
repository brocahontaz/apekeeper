package sync

import (
	"context"
	"errors"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (e Engine) capture(ctx context.Context, c domain.Character, next domain.Snapshot) error {
	prev, err := e.Stores.Progression.LastSnapshot(ctx, c.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) || domain.SnapshotChanged(prev, next) {
		return e.Stores.Progression.InsertSnapshot(ctx, next)
	}
	return nil
}
