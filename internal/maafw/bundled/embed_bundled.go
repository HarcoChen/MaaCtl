//go:build bundled

package bundled

import "embed"

// compiled marks builds that were asked to carry MaaFramework.
const compiled = true

//go:embed payload
var payloadFS embed.FS

func load() payload {
	return readPayload(payloadFS)
}
