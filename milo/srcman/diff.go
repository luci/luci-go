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
	"reflect"

	"go.chromium.org/luci/common/proto/milo"
)

// zeroCmpTwo compares two objects of the same type where the objects could be
// pointers, slices or strings.
//
// It generates a Stat value of:
//   * EQUAL if they're both nil/empty
//   * ADDED if a is nil and b is not
//   * REMOVED if b is nil and a is not
//   * For strings:
//     * EQUAL/MODIFIED depending on string equality
//   * For strings:
//     * MODIFIED if they have differing lengths
//     * Whatever the callback method returns
//   * For others
//     * Whatever the callback method returns
//
// If the callback method is considered to return MODIFIED if it is nil.
func zeroCmpTwo(a, b interface{}, modCb func() milo.ManifestDiff_Stat) milo.ManifestDiff_Stat {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	t, bt := av.Type(), bv.Type()
	if t != bt {
		panic("inconsistent types")
	}
	var az, bz bool
	if t.Kind() == reflect.Slice {
		az, bz = av.Len() == 0, bv.Len() == 0
	} else {
		az, bz = av == reflect.Zero(t), bv == reflect.Zero(t)
	}

	switch {
	case az && bz:
		return milo.ManifestDiff_EQUAL

	case !az && bz:
		return milo.ManifestDiff_ADDED

	case az && !bz:
		return milo.ManifestDiff_REMOVED

	default:
		switch t.Kind() {
		case reflect.String:
			if av.String() == bv.String() {
				return milo.ManifestDiff_EQUAL
			}
			return milo.ManifestDiff_MODIFIED

		case reflect.Slice:
			if av.Len() != bv.Len() {
				return milo.ManifestDiff_MODIFIED
			}
		}
		if modCb == nil {
			return milo.ManifestDiff_MODIFIED
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
func (c *modifiedTracker) add(st milo.ManifestDiff_Stat) milo.ManifestDiff_Stat {
	*c = *c || st != milo.ManifestDiff_EQUAL
	return st
}

// returns MODIFIED or EQUAL, depending on if the tracked state is true or
// false.
func (c modifiedTracker) status() milo.ManifestDiff_Stat {
	if c {
		return milo.ManifestDiff_MODIFIED
	}
	return milo.ManifestDiff_EQUAL
}

func diffGit(oldgit, newgit *milo.Manifest_GitCheckout) milo.ManifestDiff_Stat {
	return zeroCmpTwo(oldgit, newgit, func() milo.ManifestDiff_Stat {
		if oldgit.RepoUrl == newgit.RepoUrl {
			// FetchUrl and FetchRef don't matter for diff purposes.
			if oldgit.Revision == newgit.Revision {
				return milo.ManifestDiff_EQUAL
			}
			// DIFF indicates to the caller that the revisions are not only MODIFIED,
			// but they're able to be git diff'd.
			return milo.ManifestDiff_DIFF
		}
		return milo.ManifestDiff_MODIFIED
	})
}

func cipdPkgDiff(oldpkg, newpkg *milo.Manifest_CIPDPackage) milo.ManifestDiff_Stat {
	return zeroCmpTwo(oldpkg, newpkg, func() milo.ManifestDiff_Stat {
		// CipdVersion and CipdPackagePattern don't matter for diff purposes.
		return zeroCmpTwo(oldpkg.CipdInstanceId, newpkg.CipdInstanceId, nil)
	})
}

func isolatedDiff(oldiso, newiso []*milo.Manifest_Isolated) milo.ManifestDiff_Stat {
	return zeroCmpTwo(oldiso, newiso, func() milo.ManifestDiff_Stat {
		// we know they're non-zero and also have the same length at this point
		for i, old := range oldiso {
			new := newiso[i]
			if old.Namespace != new.Namespace {
				return milo.ManifestDiff_MODIFIED
			}
			if old.Hash != new.Hash {
				return milo.ManifestDiff_MODIFIED
			}
		}
		return milo.ManifestDiff_EQUAL
	})
}

// diffDir computes the ManifestDiff_Stat values for the two given directories.
func diffDir(olddir, newdir *milo.Manifest_Directory) *milo.ManifestDiff_Directory {
	ret := &milo.ManifestDiff_Directory{
		CipdPackage: map[string]milo.ManifestDiff_Stat{},
	}

	ret.Overall = zeroCmpTwo(olddir, newdir, func() milo.ManifestDiff_Stat {
		var dirChanged modifiedTracker
		ret.GitCheckout = dirChanged.add(diffGit(olddir.GitCheckout, newdir.GitCheckout))

		ret.CipdServerUrl = dirChanged.add(zeroCmpTwo(olddir.CipdServerUrl, newdir.CipdServerUrl, nil))
		if ret.CipdServerUrl == milo.ManifestDiff_EQUAL && newdir.CipdServerUrl != "" {
			for name, pkg := range olddir.CipdPackage {
				ret.CipdPackage[name] = dirChanged.add(cipdPkgDiff(pkg, newdir.CipdPackage[name]))
			}
			for name, pkg := range newdir.CipdPackage {
				ret.CipdPackage[name] = dirChanged.add(cipdPkgDiff(olddir.CipdPackage[name], pkg))
			}
		}

		ret.IsolatedServerUrl = dirChanged.add(zeroCmpTwo(olddir.IsolatedServerUrl, newdir.IsolatedServerUrl, nil))
		ret.Isolated = dirChanged.add(isolatedDiff(olddir.Isolated, newdir.Isolated))

		return dirChanged.status()
	})

	return ret
}

// ComputeDiff generates a Diff object which shows what changed between the `a`
// manifest and the `b` manifest.
//
// The two manifests should be ordered so that `a` is the older of the two.
func ComputeDiff(old, new *milo.Manifest) *milo.ManifestDiff {
	ret := &milo.ManifestDiff{
		Old:         old,
		New:         new,
		Directories: map[string]*milo.ManifestDiff_Directory{},
	}
	ret.Overall = zeroCmpTwo(old, new, func() milo.ManifestDiff_Stat {
		var anyChanged modifiedTracker

		for path, olddir := range old.Directories {
			dir := diffDir(olddir, new.Directories[path])
			ret.Directories[path] = dir
			anyChanged.add(dir.Overall)
		}
		for path, newdir := range new.Directories {
			dir := diffDir(old.Directories[path], newdir)
			ret.Directories[path] = dir
			anyChanged.add(dir.Overall)
		}

		return anyChanged.status()
	})
	return ret
}
