package serbian

import "embed"

//go:embed all:web
var WebFS embed.FS

//go:embed migrations
var MigrationsFS embed.FS

//go:embed prompts
var PromptsFS embed.FS
