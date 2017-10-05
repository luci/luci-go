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

package milo

import (
	"reflect"
)

// zeroCmpTwo compares two objects of the same type where the objects could be
// pointers, slices or strings.
//
// It generates a Stat value of:
//   * EQUAL if they're both nil/empty/zero
//   * ADDED if a is nil and b is not
//   * REMOVED if b is nil and a is not
//   * For strings:
//     * EQUAL/MODIFIED depending on string equality
//   * For slices:
//     * MODIFIED if they have differing lengths
//     * Whatever the callback method returns
//   * For others
//     * Whatever the callback method returns
//
// The callback method is required for non-string types.
func zeroCmpTwo(a, b interface{}, modCb func() ManifestDiff_Stat) ManifestDiff_Stat {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	t := av.Type()
	z := reflect.Zero(t)
	if t != bv.Type() {
		panic("inconsistent types")
	}
	var az, bz bool
	if t.Kind() == reflect.Slice {
		az, bz = av.Len() == 0, bv.Len() == 0
	} else {
		az, bz = av == z, bv == z
	}

	switch {
	case az && bz:
		return ManifestDiff_EQUAL

	case az && !bz:
		return ManifestDiff_ADDED

	case !az && bz:
		return ManifestDiff_REMOVED

	default:
		switch t.Kind() {
		case reflect.String:
			if av.String() == bv.String() {
				return ManifestDiff_EQUAL
			}
			return ManifestDiff_MODIFIED

		case reflect.Slice:
			if av.Len() != bv.Len() {
				return ManifestDiff_MODIFIED
			}
		}
		return modCb()
	}
}

// modifiedTracker is a simplistic structure. It:
//   * tracks the status of anything you feed to add(). If you feed a status
//     other than EQUAL, it flips to true.
//   * returns MODIFIED from status() if its bool value is true, otherwise EQUAL.
//
// It's used for semi-transparently computing `Overall` Stat values for diff
// entries containing many interesting fields.
type modifiedTracker bool

// add incorporates `st` into this modifiedTracker's state.
func (c *modifiedTracker) add(st ManifestDiff_Stat) ManifestDiff_Stat {
	*c = *c || st != ManifestDiff_EQUAL
	return st
}

// status returns MODIFIED or EQUAL, depending on if the tracked state is true
// or false.
func (c modifiedTracker) status() ManifestDiff_Stat {
	if c {
		return ManifestDiff_MODIFIED
	}
	return ManifestDiff_EQUAL
}

// Diff generates a Stat reflecting the difference between the `old`
// GitCheckout and the `new` one.
//
// This will generate a Stat of `DIFF` if the two GitCheckout's are non-nil and
// share the same RepoUrl.
//
// This only calculates the pure-data differences. Notably, this will not reach
// out to any remote services to populate the git_history field.
func (old *Manifest_GitCheckout) Diff(new *Manifest_GitCheckout) *ManifestDiff_GitCheckout {
	if old == nil && new == nil {
		return nil
	}

	ret := &ManifestDiff_GitCheckout{}

	ret.Overall = zeroCmpTwo(old, new, func() ManifestDiff_Stat {
		if old.RepoUrl == new.RepoUrl {
			// For now, the canoncial 'diff' URL is always the new URL. If we add
			// support for source-of-truth migrations, this could change.
			ret.RepoUrl = new.RepoUrl

			// FetchUrl and FetchRef don't matter for diff purposes for now.
			if old.Revision == new.Revision {
				return ManifestDiff_EQUAL
			}
			// DIFF indicates to the caller that the revisions are not only MODIFIED,
			// but they're able to be git diff'd.
			return ManifestDiff_DIFF
		}
		return ManifestDiff_MODIFIED
	})

	return ret
}

// Diff generates a ManifestDiff_Stat reflecting the difference between the `old`
// CIPDPackage and the `new` one.
func (old *Manifest_CIPDPackage) Diff(new *Manifest_CIPDPackage) ManifestDiff_Stat {
	return zeroCmpTwo(old, new, func() ManifestDiff_Stat {
		// Version and PackagePattern don't matter for diff purposes.
		return zeroCmpTwo(old.InstanceId, new.InstanceId, nil)
	})
}

// IsolatedDiff produces a ManifestDiff_Stat reflecting the difference between
// two lists of Manifest_Isolated's.
func IsolatedDiff(oldisos, newisos []*Manifest_Isolated) ManifestDiff_Stat {
	return zeroCmpTwo(oldisos, newisos, func() ManifestDiff_Stat {
		// we know they're non-zero and also have the same length at this point
		for i, old := range oldisos {
			new := newisos[i]
			if old.Namespace != new.Namespace {
				return ManifestDiff_MODIFIED
			}
			if old.Hash != new.Hash {
				return ManifestDiff_MODIFIED
			}
		}
		return ManifestDiff_EQUAL
	})
}

// Diff generates a ManifestDiff_Directory object which shows what changed
// between the `old` manifest directory and the `new` manifest directory.
func (old *Manifest_Directory) Diff(new *Manifest_Directory) *ManifestDiff_Directory {
	ret := &ManifestDiff_Directory{}

	ret.Overall = zeroCmpTwo(old, new, func() ManifestDiff_Stat {
		var dirChanged modifiedTracker

		if ret.GitCheckout = old.GitCheckout.Diff(new.GitCheckout); ret.GitCheckout != nil {
			dirChanged.add(ret.GitCheckout.Overall)
		}

		ret.CipdServerHost = dirChanged.add(zeroCmpTwo(old.CipdServerHost, new.CipdServerHost, nil))
		if ret.CipdServerHost == ManifestDiff_EQUAL && new.CipdServerHost != "" {
			cipdPackages := map[string]ManifestDiff_Stat{}

			for name, pkg := range old.CipdPackage {
				cipdPackages[name] = dirChanged.add(pkg.Diff(new.CipdPackage[name]))
			}
			for name, pkg := range new.CipdPackage {
				if _, ok := cipdPackages[name]; !ok {
					cipdPackages[name] = dirChanged.add(old.CipdPackage[name].Diff(pkg))
				}
			}

			if len(cipdPackages) > 0 {
				ret.CipdPackage = cipdPackages
			}
		}

		ret.IsolatedServerHost = dirChanged.add(zeroCmpTwo(old.IsolatedServerHost, new.IsolatedServerHost, nil))
		ret.Isolated = dirChanged.add(IsolatedDiff(old.Isolated, new.Isolated))

		return dirChanged.status()
	})

	return ret
}

// Diff generates a ManifestDiff object which shows what changed between the
// `old` manifest and the `new` manifest.
//
// This only calculates the pure-data differences. Notably, this will not reach
// out to any remote services to populate the git_history field.
func (old *Manifest) Diff(new *Manifest) *ManifestDiff {
	ret := &ManifestDiff{
		Old:         old,
		New:         new,
		Directories: map[string]*ManifestDiff_Directory{},
	}
	ret.Overall = zeroCmpTwo(old, new, func() ManifestDiff_Stat {
		var anyChanged modifiedTracker

		for path, olddir := range old.Directories {
			dir := olddir.Diff(new.Directories[path])
			ret.Directories[path] = dir
			anyChanged.add(dir.Overall)
		}
		for path, newdir := range new.Directories {
			if _, ok := ret.Directories[path]; !ok {
				dir := old.Directories[path].Diff(newdir)
				ret.Directories[path] = dir
				anyChanged.add(dir.Overall)
			}
		}

		return anyChanged.status()
	})
	return ret
}
