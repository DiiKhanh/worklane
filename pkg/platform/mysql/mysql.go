// Package mysql provides shared MySQL wiring for every service: opening a GORM
// connection and running golang-migrate migrations. It is infrastructure (pkg), so no
// domain or app code may import it - only adapters and composition roots (main.go).
package mysql

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql" // migrate mysql driver
	_ "github.com/golang-migrate/migrate/v4/source/file"    // migrate file:// source
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to MySQL via GORM. dsn is a standard go-sql-driver DSN, e.g.
// "user:pass@tcp(host:3306)/db?parseTime=true&multiStatements=true".
func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{
		// Silent: we do our own structured logging; GORM's default logger is noisy.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	return db, nil
}

// Migrate applies all up migrations found under migrationsDir (a filesystem path) to
// the database named by dsn. Migrations are the source of truth for schema - we do not
// use GORM AutoMigrate, so schema changes stay explicit and reviewable.
func Migrate(dsn, migrationsDir string) error {
	m, err := migrate.New("file://"+migrationsDir, "mysql://"+dsn)
	if err != nil {
		return fmt.Errorf("mysql: migrate init: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("mysql: migrate up: %w", err)
	}
	return nil
}
