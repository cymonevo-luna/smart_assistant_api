package migrations_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var migrationVersionRe = regexp.MustCompile(`^(\d+)_.+\.up\.(sql|json)$`)

func migrationsRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Dir(thisFile)
}

func collectMigrationVersions(t *testing.T, dir string) map[int][]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}

	versions := make(map[int][]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".up.json") {
			continue
		}
		matches := migrationVersionRe.FindStringSubmatch(name)
		if matches == nil {
			continue
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			t.Fatalf("parse version from %s: %v", name, err)
		}
		versions[version] = append(versions[version], name)
	}
	return versions
}

func assertUniqueVersions(t *testing.T, label, dir string) {
	t.Helper()
	versions := collectMigrationVersions(t, dir)
	for version, files := range versions {
		if len(files) > 1 {
			t.Fatalf("%s: migration version %d is used by multiple files: %s", label, version, strings.Join(files, ", "))
		}
	}
}

func TestMigrationVersionsUnique_Postgres(t *testing.T) {
	assertUniqueVersions(t, "postgres", migrationsRoot(t))
}

func TestMigrationVersionsUnique_Mongo(t *testing.T) {
	assertUniqueVersions(t, "mongo", filepath.Join(migrationsRoot(t), "mongo"))
}
