package buildbucket

import (
	"fmt"
)

// RietveldChange is a patchset on Rietveld.
type RietveldChange struct {
	Host     string
	Issue    int
	PatchSet int
}

// GerritChange is a patchset on gerrit.
type GerritChange struct {
	Host     string
	Change   int
	PatchSet int
}

// ParseChange tries to parse buildset as a change.
// May return RietveldChange, GerritChange or nil on failure.
func ParseChange(buildSet string) interface{} {
	var gerrit GerritChange
	if _, err := fmt.Sscanf(buildSet, "patch/gerrit/%s/%d/%d", &gerrit.Host, &gerrit.Change, &gerrit.PatchSet); err != nil {
		return gerrit
	}

	var rietveld RietveldChange
	if _, err := fmt.Sscanf(buildSet, "patch/rietveld/%s/%d/%d", &rietveld.Host, &rietveld.Issue, &rietveld.PatchSet); err != nil {
		return rietveld
	}

	return nil
}
