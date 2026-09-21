package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	Guilds      GuildStore
	Characters  CharacterStore
	Progression ProgressionStore
	SyncRuns    SyncRunStore
	Users       UserStore
}

func New(pool *pgxpool.Pool) Store {
	return Store{Guilds: GuildStore{pool}, Characters: CharacterStore{pool}, Progression: ProgressionStore{pool}, SyncRuns: SyncRunStore{pool}, Users: UserStore{pool}}
}
