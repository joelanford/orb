package catalog

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDBPath returns the default path for the catalog database.
// If ORB_DATA_DIR is set, the database is stored there; otherwise
// it falls back to <UserConfigDir>/orb.
func DefaultDBPath() (string, error) {
	if dir := os.Getenv("ORB_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "orb.db"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting user config dir: %w", err)
	}
	return filepath.Join(configDir, "orb", "orb.db"), nil
}
