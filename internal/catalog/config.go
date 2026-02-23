package catalog

import (
	"fmt"
	"os"
	"path/filepath"
)

// Catalog represents a single catalog entry.
type Catalog struct {
	Name     string            `json:"name"`
	Ref      string            `json:"ref"`
	Digest   string            `json:"digest"`
	Priority int               `json:"priority"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// DefaultDBPath returns the default path for the catalog database.
func DefaultDBPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting user config dir: %w", err)
	}
	return filepath.Join(configDir, "orb", "orb.db"), nil
}
