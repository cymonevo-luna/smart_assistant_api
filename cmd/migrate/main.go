// Command migrate applies or rolls back schema migrations using golang-migrate.
// It supports both PostgreSQL and MongoDB; the engine is selected by DB_DRIVER
// (the same variable that drives the application), so a single command works
// for whichever database the template is wired to.
//
// Usage:
//
//	go run ./cmd/migrate up          # apply all pending migrations
//	go run ./cmd/migrate down 1      # roll back one migration
//	go run ./cmd/migrate version     # print current version
//
// The database URL is taken from DB_URI (or MIGRATE_DB_URI). The migrations
// directory defaults to ./migrations for Postgres and ./migrations/mongo for
// Mongo, and can be overridden with MIGRATIONS_PATH.
package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	_ = godotenv.Load()

	if len(args) == 0 {
		return errors.New("expected a command: up | down [N] | version | force <V>")
	}

	dbURL := firstNonEmpty(os.Getenv("MIGRATE_DB_URI"), os.Getenv("DB_URI"))
	if dbURL == "" {
		return errors.New("DB_URI is required")
	}

	driver := strings.ToLower(firstNonEmpty(os.Getenv("DB_DRIVER"), "postgres"))

	source := os.Getenv("MIGRATIONS_PATH")
	if source == "" {
		source = defaultSource(driver)
	}

	if isMongo(driver) {
		// golang-migrate's mongodb driver reads the target database from the
		// connection string path, but the app's DB_URI usually omits it (the
		// database lives in MONGO_DATABASE). Splice it in when missing.
		withDB, err := ensureMongoDatabase(dbURL, firstNonEmpty(os.Getenv("MONGO_DATABASE"), "go_template"))
		if err != nil {
			return err
		}
		dbURL = withDB
	}

	return runMigrate(source, dbURL, args)
}

func isMongo(driver string) bool {
	return driver == "mongo" || driver == "mongodb"
}

func defaultSource(driver string) string {
	if isMongo(driver) {
		return "file://migrations/mongo"
	}
	return "file://migrations"
}

// ensureMongoDatabase guarantees the MongoDB connection string names a database
// in its path, which the migrate driver requires to track applied versions.
func ensureMongoDatabase(rawURL, database string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse mongo uri: %w", err)
	}
	if strings.Trim(u.Path, "/") == "" {
		u.Path = "/" + database
	}
	return u.String(), nil
}

func runMigrate(source, dbURL string, args []string) error {
	m, err := migrate.New(source, dbURL)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	switch args[0] {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		fmt.Println("migrations applied")
	case "down":
		steps := 1
		if len(args) > 1 {
			n, convErr := strconv.Atoi(args[1])
			if convErr != nil {
				return fmt.Errorf("invalid steps: %w", convErr)
			}
			steps = n
		}
		if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		fmt.Printf("rolled back %d migration(s)\n", steps)
	case "version":
		v, dirty, verErr := m.Version()
		if verErr != nil {
			return verErr
		}
		fmt.Printf("version=%d dirty=%t\n", v, dirty)
	case "force":
		if len(args) < 2 {
			return errors.New("force requires a version")
		}
		v, convErr := strconv.Atoi(args[1])
		if convErr != nil {
			return convErr
		}
		if err := m.Force(v); err != nil {
			return err
		}
		fmt.Printf("forced version %d\n", v)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
