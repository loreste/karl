//go:build !opus

package internal

// OpusAvailable indicates whether real Opus codec support is compiled in.
// Build with `-tags opus` and have libopus + libopusfile installed for real Opus.
const OpusAvailable = false
