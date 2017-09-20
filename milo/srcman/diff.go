// Copyright 2017 The LUCI Authors.
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

package srcman

import (
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/data/stringset"
	srcman_pb "go.chromium.org/luci/milo/api/proto/manifest"
)

// Stat is a simple enumeration classifying the type of change present in a Diff
// or a DirectoryDiff.
type Stat int

// The types of status indicators for a Diff.
const (
	EQUAL Stat = iota
	ADDED
	REMOVED
	MODIFIED
)

// Diff represents the difference between two Manifest protos.
type Diff struct {
	A, B *srcman_pb.Manifest

	// OverallChange is true if A and B differ in some fashion.
	OverallChange bool

	// VersionChange is true if the Version field differs between A and B.
	VersionChange        bool
	ChangedDirectories   map[string]Stat
	UnchangedDirectories stringset.Set
}

// ComputeDiff generates a Diff object which shows what changed between the `a`
// manifest and the `b` manifest.
//
// The two manifests should be ordered so that `a` is the older of the two.
func ComputeDiff(a, b *srcman_pb.Manifest) *Diff {
	ret := &Diff{
		A: a, B: b,
		VersionChange: a.Version != b.Version,
	}

	unchanged := []string{}

	allDirectories := stringset.New(len(a.Directories) + len(b.Directories))
	for k := range a.Directories {
		allDirectories.Add(k)
	}
	for k := range b.Directories {
		allDirectories.Add(k)
	}

	allDirectories.Iter(func(dir string) bool {
		aDir, bDir := a.Directories[dir], b.Directories[dir]
		action := EQUAL
		switch {
		case aDir == nil:
			action = ADDED
		case bDir == nil:
			action = REMOVED
		case !proto.Equal(aDir, bDir):
			action = MODIFIED
		}

		if action == EQUAL {
			unchanged = append(unchanged, dir)
		} else {
			if ret.ChangedDirectories == nil {
				ret.ChangedDirectories = map[string]Stat{}
			}
			ret.ChangedDirectories[dir] = action
		}

		return true
	})

	ret.OverallChange = len(ret.ChangedDirectories) > 0 || ret.VersionChange
	ret.UnchangedDirectories = stringset.NewFromSlice(unchanged...)

	return ret
}
