// Package migrations embeds the application schema so the desktop sidecar is
// self-contained and does not need a migrations directory at runtime.
package migrations

import "embed"

// Files contains every SQL migration at the root of this package.
//
//go:embed *.sql
var Files embed.FS
