package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib" // ← этот импорт регистрирует драйвер "pgx"

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
