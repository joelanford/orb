package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Catalog represents a single catalog entry in the config file.
type Catalog struct {
	Name       string `json:"name"`
	Ref        string `json:"ref"`
	ContentDir string `json:"contentDir"`
	Priority   int    `json:"priority"`
}

// Config holds the list of configured catalogs.
type Config struct {
	Catalogs []Catalog `json:"catalogs"`
}

// DefaultConfigPath returns the default path for the catalogs config file.
func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting user config dir: %w", err)
	}
	return filepath.Join(configDir, "orb", "catalogs.json"), nil
}

// DefaultCacheDir returns the default cache directory for catalog content.
func DefaultCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("getting user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "orb", "catalogs"), nil
}

// Load reads the config from the given path. If the file does not exist,
// an empty config is returned.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to the given path atomically, creating parent
// directories as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Add appends a catalog entry, returning an error if a catalog with the
// same name already exists.
func (c *Config) Add(cat Catalog) error {
	if _, ok := c.Get(cat.Name); ok {
		return fmt.Errorf("catalog %q already exists", cat.Name)
	}
	c.Catalogs = append(c.Catalogs, cat)
	return nil
}

// Remove finds and removes a catalog by name, returning the removed entry.
func (c *Config) Remove(name string) (Catalog, error) {
	for i, cat := range c.Catalogs {
		if cat.Name == name {
			c.Catalogs = append(c.Catalogs[:i], c.Catalogs[i+1:]...)
			return cat, nil
		}
	}
	return Catalog{}, fmt.Errorf("catalog %q not found", name)
}

// Get looks up a catalog by name.
func (c *Config) Get(name string) (*Catalog, bool) {
	for i := range c.Catalogs {
		if c.Catalogs[i].Name == name {
			return &c.Catalogs[i], true
		}
	}
	return nil, false
}
