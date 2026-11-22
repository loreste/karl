//go:build !amr

package internal

// AMRAvailable indicates whether real AMR codec support is compiled in.
// Build with `-tags amr` and have opencore-amrnb installed for real AMR.
const AMRAvailable = false
