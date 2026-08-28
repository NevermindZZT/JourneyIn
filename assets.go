package journeyin

import "embed"

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
