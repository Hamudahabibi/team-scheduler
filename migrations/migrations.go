// Package migrations встраивает SQL-миграции в бинарник.
package migrations

import "embed"

// FS — файлы миграций в формате goose.
//
//go:embed *.sql
var FS embed.FS
