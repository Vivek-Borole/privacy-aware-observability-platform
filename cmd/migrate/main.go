package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	databaseURL := os.Getenv("PAOP_POSTGRES_URL")
	if databaseURL == "" {
		log.Fatal("PAOP_POSTGRES_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `create table if not exists schema_migrations (version text primary key, applied_at timestamptz not null default now())`); err != nil {
		log.Fatal(err)
	}
	files, err := filepath.Glob("db/migrations/*.sql")
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		version := filepath.Base(file)
		var exists bool
		if err := db.QueryRowContext(ctx, `select exists(select 1 from schema_migrations where version = $1)`, version).Scan(&exists); err != nil {
			log.Fatal(err)
		}
		if exists {
			continue
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			tx.Rollback()
			log.Fatalf("migration %s: %v", version, err)
		}
		if _, err := tx.ExecContext(ctx, `insert into schema_migrations(version) values($1)`, version); err != nil {
			tx.Rollback()
			log.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			log.Fatal(err)
		}
		log.Printf("applied %s", version)
	}
}
