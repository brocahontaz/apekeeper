package migrations

import "embed"

// FS contains ordered SQL migrations.
//
//go:embed *.sql
var FS embed.FS
