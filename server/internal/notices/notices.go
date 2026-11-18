// Package notices embeds the license notices yume-server must carry: the
// data and dictionary credits of LICENSES.md, and the license of every Go
// module and bundled npm package. Regenerate THIRD_PARTY_NOTICES.txt with
// `node scripts/third-party-notices.mjs` from the repo root; CI fails when
// it is stale.
package notices

import _ "embed"

// Text is served at /licenses.
//
//go:embed THIRD_PARTY_NOTICES.txt
var Text string
