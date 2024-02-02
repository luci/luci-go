// Copyright 2020 The LUCI Authors.
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

package common

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth"
)

// RunKind is the Datastore entity kind for Run.
const RunKind = "Run"

// MaxRunTotalDuration is the max total duration of the Run.
//
// Total duration means end time - create time. Run will be cancelled after
// the total duration is reached.
const MaxRunTotalDuration = 10 * 24 * time.Hour // 10 days

// RunID is an unique RunID to identify a Run in CV.
//
// RunID is string like `luciProject/inverseTS-1-hexHashDigest` consisting of
// 7 parts:
//  1. The LUCI Project that this Run belongs to.
//     Purpose: separates load on Datastore from different projects.
//  2. `/` separator.
//  3. InverseTS, defined as (`endOfTheWorld` - CreateTime) in ms precision,
//     left-padded with zeros to 13 digits. See `Run.CreateTime` Doc.
//     Purpose: ensures queries by default orders runs of the same project by
//     most recent first.
//  4. `-` separator.
//  5. Digest version (see part 7).
//  6. `-` separator.
//  7. A hex digest string uniquely identifying the set of CLs involved in
//     this Run.
//     Purpose: ensures two simultaneously started Runs in the same project
//     won't have the same RunID.
type RunID string

// CV will be dead on ~292.3 years after first LUCI design doc was created.
//
// Computed as https://play.golang.com/p/hDQ-EhlSLu5
//
//	luci := time.Date(2014, time.May, 9, 1, 26, 0, 0, time.UTC)
//	endOfTheWorld := luci.Add(time.Duration(1<<63 - 1))
var endOfTheWorld = time.Date(2306, time.August, 19, 1, 13, 16, 854775807, time.UTC)

func MakeRunID(luciProject string, createTime time.Time, digestVersion int, clsDigest []byte) RunID {
	if endOfTheWorld.Sub(createTime) == 1<<63-1 {
		panic(fmt.Errorf("overflow"))
	}
	ms := endOfTheWorld.Sub(createTime).Milliseconds()
	if ms < 0 {
		panic(fmt.Errorf("Can't create run at %s which is after endOfTheWorld %s", createTime, endOfTheWorld))
	}
	id := fmt.Sprintf("%s/%013d-%d-%s", luciProject, ms, digestVersion, hex.EncodeToString(clsDigest))
	return RunID(id)
}

// Validate returns an error if Run ID is not valid.
//
// If validate returns nil,
//   - it means all other methods on RunID will work fine instead of panicking,
//   - it doesn't mean Run ID is possible to generate using the MakeRunID.
//     This is especially relevant in CV tests, where specifying short Run IDs is
//     useful.
func (id RunID) Validate() (err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "malformed RunID %q", id).Err()
		}
	}()

	allDigits := func(digits string) bool {
		for _, r := range digits {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}

	s := string(id)
	i := strings.IndexRune(s, '/')
	if i < 1 {
		return fmt.Errorf("lacks LUCI project")
	}
	if err := config.ValidateProjectName(s[:i]); err != nil {
		return fmt.Errorf("invalid LUCI project part: %s", err)
	}
	s = s[i+1:]

	i = strings.IndexRune(s, '-')
	if i < 1 {
		return fmt.Errorf("lacks InverseTS part")
	}
	if !allDigits(s[:i]) {
		return fmt.Errorf("invalid InverseTS")
	}

	s = s[i+1:]
	i = strings.IndexRune(s, '-')
	if i < 1 {
		return fmt.Errorf("lacks version")
	}
	if !allDigits(s[:i]) {
		return fmt.Errorf("invalid version")
	}

	s = s[i+1:]
	if len(s) == 0 {
		return fmt.Errorf("lacks digest")
	}
	return nil
}

// LUCIProject this Run belongs to.
func (id RunID) LUCIProject() string {
	pos := strings.IndexRune(string(id), '/')
	if pos == -1 {
		panic(fmt.Errorf("invalid run ID %q", id))
	}
	return string(id[:pos])
}

// Inner is the part after "<LUCIProject>/" for use in UI.
func (id RunID) Inner() string {
	pos := strings.IndexRune(string(id), '/')
	if pos == -1 {
		panic(fmt.Errorf("invalid run ID %q", id))
	}
	return string(id[pos+1:])
}

// InverseTS of this Run. See RunID doc.
func (id RunID) InverseTS() string {
	s := string(id)
	posSlash := strings.IndexRune(s, '/')
	if posSlash == -1 {
		panic(fmt.Errorf("invalid run ID %q", id))
	}
	s = s[posSlash+1:]
	posDash := strings.IndexRune(s, '-')
	return s[:posDash]
}

// PublicID returns the public representation of the RunID.
//
// The format of a public ID is `projects/$luci-project/runs/$id`, where
// - luci-project is the name of the LUCI project the Run belongs to
// - id is an opaque key unique in the LUCI project.
func (id RunID) PublicID() string {
	prj := id.LUCIProject()
	return fmt.Sprintf("projects/%s/runs/%s", prj, string(id[len(prj)+1:]))
}

// FromPublicRunID is the inverse of RunID.PublicID().
func FromPublicRunID(id string) (RunID, error) {
	parts := strings.Split(id, "/")
	if len(parts) == 4 && parts[0] == "projects" && parts[2] == "runs" {
		return RunID(parts[1] + "/" + parts[3]), nil
	}
	return "", errors.Reason(`Run ID must be in the form "projects/$luci-project/runs/$id", but %q given"`, id).Err()
}

// AttemptKey returns CQDaemon attempt key.
func (id RunID) AttemptKey() string {
	i := strings.LastIndexByte(string(id), '-')
	if i == -1 || i == len(id)-1 {
		panic(fmt.Errorf("invalid run ID %q", id))
	}
	return string(id[i+1:])
}

// RunIDs is a convenience type to facilitate handling of run RunIDs.
type RunIDs []RunID

// sort.Interface copy-pasta.
func (ids RunIDs) Less(i, j int) bool { return ids[i] < ids[j] }
func (ids RunIDs) Len() int           { return len(ids) }
func (ids RunIDs) Swap(i, j int)      { ids[i], ids[j] = ids[j], ids[i] }

// WithoutSorted returns a subsequence of IDs without excluded IDs.
//
// Both this and the excluded slices must be sorted.
//
// If this and excluded IDs are disjoint, return this slice.
// Otherwise, returns a copy without excluded IDs.
func (ids RunIDs) WithoutSorted(exclude RunIDs) RunIDs {
	remaining := ids
	ret := ids
	mutated := false
	for {
		switch {
		case len(remaining) == 0:
			return ret
		case len(exclude) == 0:
			if mutated {
				ret = append(ret, remaining...)
			}
			return ret
		case remaining[0] < exclude[0]:
			if mutated {
				ret = append(ret, remaining[0])
			}
			remaining = remaining[1:]
		case remaining[0] > exclude[0]:
			exclude = exclude[1:]
		default:
			if !mutated {
				// Must copy all IDs that were skipped.
				mutated = true
				n := len(ids) - len(remaining)
				ret = make(RunIDs, n, len(ids)-1)
				copy(ret, ids) // copies len(ret) == n elements.
			}
			remaining = remaining[1:]
			exclude = exclude[1:]
		}
	}
}

// InsertSorted adds given ID if not yet exists to the list keeping list sorted.
//
// InsertSorted is a pointer receiver method, because it modifies slice itself.
func (p *RunIDs) InsertSorted(id RunID) {
	ids := *p
	switch i := sort.Search(len(ids), func(i int) bool { return ids[i] >= id }); {
	case i == len(ids):
		*p = append(ids, id)
	case ids[i] > id:
		// Insert new ID at position i and shift the rest of slice to the right.
		toInsert := id
		for ; i < len(ids); i++ {
			ids[i], toInsert = toInsert, ids[i]
		}
		*p = append(ids, toInsert)
	}
}

// DelSorted removes the given ID if it exists.
//
// DelSorted is a pointer receiver method, because it modifies slice itself.
func (p *RunIDs) DelSorted(id RunID) bool {
	ids := *p
	i := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	if i == len(ids) || ids[i] != id {
		return false
	}

	copy(ids[i:], ids[i+1:])
	ids[len(ids)-1] = ""
	*p = ids[:len(ids)-1]
	return true
}

// ContainsSorted returns true if ids contain the given one.
func (ids RunIDs) ContainsSorted(id RunID) bool {
	i := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	return i < len(ids) && ids[i] == id
}

// DifferenceSorted returns all IDs in this slice and not the other one.
//
// Both slices must be sorted. Doesn't modify input slices.
func (a RunIDs) DifferenceSorted(b RunIDs) RunIDs {
	var diff RunIDs
	for {
		if len(b) == 0 {
			return append(diff, a...)
		}
		if len(a) == 0 {
			return diff
		}
		x, y := a[0], b[0]
		switch {
		case x == y:
			a, b = a[1:], b[1:]
		case x < y:
			diff = append(diff, x)
			a = a[1:]
		default:
			b = b[1:]
		}
	}
}

// Index returns the index of the first instance of the provided id.
//
// Returns -1 if the provided id isn't present.
func (ids RunIDs) Index(target RunID) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

// Equal checks if two ids are equal.
func (ids RunIDs) Equal(other RunIDs) bool {
	if len(ids) != len(other) {
		return false
	}
	for i, id := range ids {
		if id != other[i] {
			return false
		}
	}
	return true
}

// Set returns a new set of run IDs.
func (ids RunIDs) Set() map[RunID]struct{} {
	r := make(map[RunID]struct{}, len(ids))
	for _, id := range ids {
		r[id] = struct{}{}
	}
	return r
}

// MakeRunIDs returns RunIDs from list of strings.
func MakeRunIDs(ids ...string) RunIDs {
	ret := make(RunIDs, len(ids))
	for i, id := range ids {
		ret[i] = RunID(id)
	}
	return ret
}

// MCEDogfooderGroup is a CrIA group who signed up for dogfooding MCE.
const MCEDogfooderGroup = "luci-cv-mce-dogfooders"

// IsMCEDogfooder returns true if the user is an MCE dogfooder.
//
// TODO(ddoman): remove this function, once MCE dogfood is done.
func IsMCEDogfooder(ctx context.Context, id identity.Identity) bool {
	// if it fails to retrieve the authDB, then log the error and return false.
	// this function will be removed, anyways.
	ret, err := auth.GetState(ctx).DB().IsMember(ctx, id, []string{MCEDogfooderGroup})
	if err != nil {
		logging.Errorf(ctx, "IsMCEDogfooder: auth.IsMember: %s", err)
	}
	return ret
}

// InstantTriggerDogfooderGroup is the CrIA group who signed up for dogfooding
// cros instant trigger.
const InstantTriggerDogfooderGroup = "luci-cv-instant-trigger-dogfooders"

// IsInstantTriggerDogfooder returns true if the given user participate in
// the cros instant trigger dogfood.
//
// TODO(yiwzhang): remove this function, once cros instant trigger dogfood is
// done.
func IsInstantTriggerDogfooder(ctx context.Context, id identity.Identity) bool {
	// if it fails to retrieve the authDB, then log the error and return false.
	// this function will be removed, anyways.
	ret, err := auth.GetState(ctx).DB().IsMember(ctx, id, []string{InstantTriggerDogfooderGroup})
	if err != nil {
		logging.Errorf(ctx, "IsInstantTriggerDogfooder: auth.IsMember: %s", err)
		return false
	}
	return ret
}
