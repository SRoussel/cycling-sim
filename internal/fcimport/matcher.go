package fcimport

import "strings"

// MatchRiderByName attempts to find an internal rider ID from the provided
// name→id map by trying several normalization strategies.
//
// The map is expected to have lowercase full names as keys:
//
//	"tadej pogacar" → 42
//	"wout van aert" → 17
//
// Returns (id, true) if a match is found.
func MatchRiderByName(normalizedName string, nameToID map[string]int64) (int64, bool) {
	key := strings.ToLower(strings.TrimSpace(normalizedName))
	if id, ok := nameToID[key]; ok {
		return id, true
	}
	// Try matching with collapsed whitespace
	key = strings.Join(strings.Fields(key), " ")
	if id, ok := nameToID[key]; ok {
		return id, true
	}
	return 0, false
}

// BuildNameMap constructs a lowercase-name → rider-ID lookup from a slice of
// (id, name) pairs as returned by ListRiderNamesAndIDs.
func BuildNameMap(pairs []RiderNameID) map[string]int64 {
	m := make(map[string]int64, len(pairs))
	for _, p := range pairs {
		m[strings.ToLower(p.Name)] = p.ID
	}
	return m
}

// RiderNameID is a lightweight struct used to build the name→ID lookup.
type RiderNameID struct {
	ID   int64
	Name string
}
