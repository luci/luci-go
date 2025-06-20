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

// Package spec contains implementation for search, parse, normalize vpython
// spec.
package spec

import (
	"sort"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/vpython/api/vpython"
)

// NormalizeSpec normalizes the specification Message such that two messages
// with identical meaning will have identical representation.
//
// If multiple wheel entries exist for the same package name, they must also
// share a version. If they don't, an error will be returned. Otherwise, they
// will be merged into a single wheel entry.
//
// NormalizeSpec will prune any Wheel entries that don't match the specified
// tags, and will remove the match entries from any remaining Wheel entries.
func NormalizeSpec(spec *vpython.Spec, tags []*vpython.PEP425Tag) error {
	// Apply match filters, prune any entries that don't match, and clear the
	// MatchTag entries for those that do.
	//
	// Make sure the VirtualEnv package isn't listed in the wheels list.
	//
	// Track the versions for each package and assert that any duplicate packages
	// don't share a version.
	pos := 0
	packageVersions := make(map[string]string, len(spec.Wheel))
	for _, w := range spec.Wheel {
		// If this package doesn't match the tag set, skip it.
		if !PackageMatches(w, tags) {
			continue
		}

		// If this package has already been included, assert version consistency.
		if v, ok := packageVersions[w.Name]; ok {
			if v != w.Version {
				return errors.Fmt("multiple versions for package %q: %q != %q", w.Name, w.Version, v)
			}

			// This package has already been included, so we can ignore it.
			continue
		}

		// Mark that this package was included, so we can assert version consistency
		// and avoid duplicates.
		packageVersions[w.Name] = w.Version
		w.MatchTag = nil
		spec.Wheel[pos] = w
		pos++
	}
	spec.Wheel = spec.Wheel[:pos]
	sort.Sort(specPackageSlice(spec.Wheel))
	return nil
}

type specPackageSlice []*vpython.Spec_Package

func (s specPackageSlice) Len() int      { return len(s) }
func (s specPackageSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s specPackageSlice) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return s[i].Name < s[j].Name },
		func(i, j int) bool { return s[i].Version < s[j].Version },
	}.Use(i, j)
}
