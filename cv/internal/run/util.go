// Copyright 2021 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package run

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
)

// HasRootCL returns true if this Run has root CL specified.
//
// It means this run is instantly triggered Run in combined CLs mode (i.e.
// multi-Cl mode).
func (r *Run) HasRootCL() bool {
	return r.RootCL != 0
}

// ComputeCLGroupKey constructs keys for ClGroupKey and the related
// EquivalentClGroupKey.
//
// These are meant to be opaque keys unique to particular set of CLs and
// patchsets for the purpose of grouping together Runs for the same sets of
// patchsets. if isEquivalent is true, then the "min equivalent patchset" is
// used instead of the latest patchset, so that trivial patchsets such as minor
// rebases and CL description updates don't change the key.
func ComputeCLGroupKey(cls []*RunCL, isEquivalent bool) string {
	sort.Slice(cls, func(i, j int) bool {
		// ExternalID includes host and change number but not patchset; but
		// different patchsets of the same CL will never be included in the
		// same list, so sorting on only ExternalID is sufficient.
		return cls[i].ExternalID < cls[j].ExternalID
	})
	h := sha256.New()
	// CL group keys are meant to be opaque keys. We'd like to avoid people
	// depending on CL group key and equivalent CL group key sometimes being
	// equal. We can do this by adding a salt to the hash.
	if isEquivalent {
		h.Write([]byte("equivalent_cl_group_key"))
	}
	separator := []byte{0}
	for i, cl := range cls {
		if i > 0 {
			h.Write(separator)
		}
		h.Write([]byte(cl.Detail.GetGerrit().GetHost()))
		h.Write(separator)
		h.Write([]byte(strconv.FormatInt(cl.Detail.GetGerrit().GetInfo().GetNumber(), 10)))
		h.Write(separator)
		if isEquivalent {
			h.Write([]byte(strconv.FormatInt(int64(cl.Detail.GetMinEquivalentPatchset()), 10)))
		} else {
			h.Write([]byte(strconv.FormatInt(int64(cl.Detail.GetPatchset()), 10)))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// ShouldSubmit returns true if the run should submit the CL(s) at the end.
func ShouldSubmit(r *Run) bool {
	return r.Mode == FullRun || r.ModeDefinition.GetCqLabelValue() == 2
}
