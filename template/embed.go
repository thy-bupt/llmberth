// Package template embeds the generated-application template sources.
//
// The actual template files live under app/ and are embedded at build time.
// Files ending in .tmpl are rendered via text/template; everything else is
// copied verbatim. Directory names wrapped in double underscores (e.g.
// __ui__) are conditional: they are skipped unless the matching option is
// enabled, and the wrapper is stripped in the output path.
package template

import "embed"

// FS is the embedded template source tree (app/).
//
//go:embed all:app
var FS embed.FS

// Root is the FS sub-directory holding the template sources.
const Root = "app"
