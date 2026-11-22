//go:build !g729

package internal

// G729Available indicates whether real G.729 codec support is compiled in.
// Build with `-tags g729` and have libbcg729 installed for real G.729.
const G729Available = false
