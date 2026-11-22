package commands

import (
	"strings"

	"karl/internal"
)

// negotiateCodecs implements RFC 3264 codec negotiation.
// It returns the intersection of the offerer's and answerer's codec lists,
// ordered by the offerer's preference (the offerer's order is authoritative).
func negotiateCodecs(offererCodecs, answererCodecs []internal.CodecInfo) []internal.CodecInfo {
	if len(offererCodecs) == 0 || len(answererCodecs) == 0 {
		return answererCodecs
	}

	// Build a set of answerer codec names for fast lookup
	answererSet := make(map[string]internal.CodecInfo, len(answererCodecs))
	for _, c := range answererCodecs {
		answererSet[strings.ToLower(c.Name)] = c
	}

	// Walk the offerer's list in order, keep only codecs the answerer also supports
	result := make([]internal.CodecInfo, 0, len(offererCodecs))
	for _, offered := range offererCodecs {
		if answered, ok := answererSet[strings.ToLower(offered.Name)]; ok {
			// Use the answerer's payload type (it may differ) but keep offerer's order
			result = append(result, answered)
		}
	}

	// If intersection is empty, fall back to answerer's full list to avoid zero-codec SDP
	if len(result) == 0 {
		return answererCodecs
	}

	return result
}
