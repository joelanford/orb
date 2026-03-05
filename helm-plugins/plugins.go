package helmplugins

import "embed"

//go:embed */plugin.yaml */run.sh
var FS embed.FS
