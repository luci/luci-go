// Copyright 2026 The LUCI Authors.
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

// Package version provides utilities for parsing and comparing numerical
// software versions.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a 4-part numerical version (Major.Minor.Patch.Build).
type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
	Build uint64
}

// Parse parses a version string with up to 4 dot-separated components.
// Missing components default to 0.
func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return Version{}, fmt.Errorf("%q has more than 4 components", s)
	}

	nums := [4]uint64{}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("parsing component %d (%q): %w", i, part, err)
		}
		nums[i] = n
	}

	return Version{
		Major: nums[0],
		Minor: nums[1],
		Patch: nums[2],
		Build: nums[3],
	}, nil
}

// MustParse parses a version string and panics if parsing fails.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// Compare compares v to other. It returns:
//
//	-1 if v < other
//	 0 if v == other
//	+1 if v > other
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	if v.Build != other.Build {
		if v.Build < other.Build {
			return -1
		}
		return 1
	}
	return 0
}

// Less reports whether v is strictly smaller than other.
func (v Version) Less(other Version) bool {
	return v.Compare(other) < 0
}

// String returns the canonical representation "MAJOR.MINOR.PATCH.BUILD".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Patch, v.Build)
}
