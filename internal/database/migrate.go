package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/pressly/goose"
)

func RunMigrations(dsn, migrationsDir string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("❌ error opened BD for migration: %w", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("❌ error migration:%w", err)
	}

	log.Println("✅ migration applied succesfull!")
	return nil
}
