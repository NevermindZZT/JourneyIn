package journeyin

import (
	"embed"
	"strings"
)

// Version is the application release version from the repository VERSION file.
// Release builds may still override the command's copy with -ldflags.
//
//go:embed VERSION
var versionFile string

var Version = strings.TrimSpace(versionFile)

// WebFS contains the production Web/PWA bundle.
//
//go:embed all:web/dist
var WebFS embed.FS

// SchemaFS contains versioned interchange schemas.
//
//go:embed all:schemas
var SchemaFS embed.FS

// MigrationFS contains embedded database migrations.
//
//go:embed all:migrations
var MigrationFS embed.FS
