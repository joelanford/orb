package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const createTablesSQL = `
CREATE TABLE IF NOT EXISTS catalogs (
    name     TEXT PRIMARY KEY,
    ref      TEXT NOT NULL,
    digest   TEXT NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 0,
    labels   TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS packages (
    catalog_name TEXT NOT NULL REFERENCES catalogs(name) ON DELETE CASCADE,
    package_name TEXT NOT NULL,
    data         TEXT NOT NULL,
    PRIMARY KEY (catalog_name, package_name)
);
`

// DB wraps a SQLite database for catalog and package data storage.
type DB struct {
	db *sql.DB
}

// OpenDB opens (or creates) a SQLite database at the given path.
// It creates tables if they do not exist and enables foreign keys.
func OpenDB(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating DB directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Limit to one connection — modernc.org/sqlite has internal Go maps
	// that are not safe for concurrent use across multiple connections.
	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("setting journal mode: %w", err)
	}

	if _, err := sqlDB.Exec(createTablesSQL); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	return &DB{db: sqlDB}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// AddCatalog inserts a new catalog entry. Returns an error if a catalog
// with the same name already exists.
func (d *DB) AddCatalog(cat Catalog) error {
	labelsJSON := marshalLabels(cat.Labels)
	_, err := d.db.Exec(
		"INSERT INTO catalogs (name, ref, digest, priority, labels) VALUES (?, ?, ?, ?, ?)",
		cat.Name, cat.Ref, cat.Digest, cat.Priority, labelsJSON,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("catalog %q already exists", cat.Name)
		}
		return fmt.Errorf("inserting catalog: %w", err)
	}
	return nil
}

// RemoveCatalog removes a catalog by name, returning the removed entry.
// CASCADE automatically removes associated package data.
func (d *DB) RemoveCatalog(name string) (Catalog, error) {
	cat, err := d.GetCatalog(name)
	if err != nil {
		return Catalog{}, err
	}
	if cat == nil {
		return Catalog{}, fmt.Errorf("catalog %q not found", name)
	}

	if _, err := d.db.Exec("DELETE FROM catalogs WHERE name = ?", name); err != nil {
		return Catalog{}, fmt.Errorf("deleting catalog: %w", err)
	}
	return *cat, nil
}

// GetCatalog looks up a catalog by name. Returns nil if not found.
func (d *DB) GetCatalog(name string) (*Catalog, error) {
	var cat Catalog
	var labelsJSON string
	err := d.db.QueryRow(
		"SELECT name, ref, digest, priority, labels FROM catalogs WHERE name = ?", name,
	).Scan(&cat.Name, &cat.Ref, &cat.Digest, &cat.Priority, &labelsJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying catalog: %w", err)
	}
	cat.Labels = unmarshalLabels(labelsJSON)
	return &cat, nil
}

// UpdateCatalog reads a catalog entry by name, passes it to fn for
// modification, and writes it back. Returns an error if not found.
func (d *DB) UpdateCatalog(name string, fn func(*Catalog)) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var cat Catalog
	var labelsJSON string
	err = tx.QueryRow(
		"SELECT name, ref, digest, priority, labels FROM catalogs WHERE name = ?", name,
	).Scan(&cat.Name, &cat.Ref, &cat.Digest, &cat.Priority, &labelsJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("catalog %q not found", name)
		}
		return fmt.Errorf("reading catalog: %w", err)
	}
	cat.Labels = unmarshalLabels(labelsJSON)

	fn(&cat)

	updatedLabels := marshalLabels(cat.Labels)
	_, err = tx.Exec(
		"UPDATE catalogs SET ref = ?, digest = ?, priority = ?, labels = ? WHERE name = ?",
		cat.Ref, cat.Digest, cat.Priority, updatedLabels, name,
	)
	if err != nil {
		return fmt.Errorf("updating catalog: %w", err)
	}

	return tx.Commit()
}

// SortedCatalogs returns all catalogs sorted by priority descending,
// then by name ascending.
func (d *DB) SortedCatalogs() ([]Catalog, error) {
	rows, err := d.db.Query(
		"SELECT name, ref, digest, priority, labels FROM catalogs ORDER BY priority DESC, name ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}
	defer rows.Close()

	var catalogs []Catalog
	for rows.Next() {
		var cat Catalog
		var labelsJSON string
		if err := rows.Scan(&cat.Name, &cat.Ref, &cat.Digest, &cat.Priority, &labelsJSON); err != nil {
			return nil, fmt.Errorf("scanning catalog row: %w", err)
		}
		cat.Labels = unmarshalLabels(labelsJSON)
		catalogs = append(catalogs, cat)
	}
	return catalogs, rows.Err()
}

// SetPackageData replaces all package data for a catalog in a transaction.
func (d *DB) SetPackageData(catalogName string, pkgs map[string]*PackageData) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM packages WHERE catalog_name = ?", catalogName); err != nil {
		return fmt.Errorf("deleting existing packages: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO packages (catalog_name, package_name, data) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	for name, data := range pkgs {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshaling package data for %q: %w", name, err)
		}
		if _, err := stmt.Exec(catalogName, name, string(jsonData)); err != nil {
			return fmt.Errorf("inserting package %q: %w", name, err)
		}
	}

	return tx.Commit()
}

// GetPackageData retrieves package data for a specific package in a catalog.
// Returns nil if the package is not found.
func (d *DB) GetPackageData(catalogName, packageName string) (*PackageData, error) {
	var dataJSON string
	err := d.db.QueryRow(
		"SELECT data FROM packages WHERE catalog_name = ? AND package_name = ?",
		catalogName, packageName,
	).Scan(&dataJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying package data: %w", err)
	}

	var pd PackageData
	if err := json.Unmarshal([]byte(dataJSON), &pd); err != nil {
		return nil, fmt.Errorf("unmarshaling package data: %w", err)
	}
	return &pd, nil
}

func marshalLabels(labels map[string]string) string {
	if labels == nil {
		return "{}"
	}
	data, _ := json.Marshal(labels)
	return string(data)
}

func unmarshalLabels(s string) map[string]string {
	var labels map[string]string
	_ = json.Unmarshal([]byte(s), &labels)
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func isUniqueViolation(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) ||
		// modernc.org/sqlite returns constraint errors as string messages
		containsString(err.Error(), "UNIQUE constraint failed") ||
		containsString(err.Error(), "constraint failed"))
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
