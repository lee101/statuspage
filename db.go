package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func openDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func runMigrations(db *sql.DB) error {
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func loadDotEnv(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || os.Getenv(strings.TrimSpace(k)) != "" {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"`)
		v = strings.Trim(v, `'`)
		os.Setenv(strings.TrimSpace(k), v)
	}
}
