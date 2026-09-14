//go:build !bundled

package bundled

// compiled marks builds that were asked to carry MaaFramework.
const compiled = false

func load() payload {
	return payload{}
}
