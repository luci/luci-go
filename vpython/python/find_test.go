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

package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.chromium.org/luci/common/system/filesystem"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFind(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		version      Version
		interpreters []string

		err          string
		found        string
		foundVersion Version
	}{
		{
			version:      Version{Major: 2},
			interpreters: []string{"python1", "python2", "python3"},
			found:        "python2",
			foundVersion: Version{Major: 2},
		},

		{
			version:      Version{Major: 2, Minor: 7, Patch: 12},
			interpreters: []string{"python2", "python2.6", "python2.7", "python3"},
			found:        "python2.7",
			foundVersion: Version{Major: 2, Minor: 7, Patch: 14},
		},

		{
			version:      Version{Major: 2},
			interpreters: []string{"python", "python3", "python3.6"},
			foundVersion: Version{Major: 3, Minor: 6},
			err:          "no Python found",
		},
	}

	Convey(`Can Find a Python interpreter`, t, func() {
		tdir := t.TempDir()
		c := context.Background()

		var lookPathVersion Version
		testLookPath := func(c context.Context, target string, filter LookPathFilter) (*LookPathResult, error) {
			path := filepath.Join(tdir, target)
			if _, err := os.Stat(path); err != nil {
				return nil, err
			}
			i := Interpreter{
				Python: path,
			}
			i.cachedVersion = &lookPathVersion
			if err := filter(c, &i); err != nil {
				return nil, err
			}
			return &LookPathResult{Path: path, Version: lookPathVersion}, nil
		}

		for i, tc := range testCases {
			kind := "success"
			if tc.err != "" {
				kind = "failure"
			}

			Convey(fmt.Sprintf(`Test case #%d (%s): find %q in %v`, i, kind, tc.version, tc.interpreters), func() {
				for _, interpreter := range tc.interpreters {
					path := filepath.Join(tdir, interpreter)
					if err := filesystem.Touch(path, time.Time{}, 0644); err != nil {
						t.Fatalf("could not create interpreter %q: %s", path, err)
					}
				}

				lookPathVersion = tc.foundVersion
				interp, err := Find(c, tc.version, testLookPath)
				if tc.err == "" {
					So(err, ShouldBeNil)

					// Success case. Version will be cached.
					version, err := interp.GetVersion(c)
					So(err, ShouldBeNil)
					So(filepath.Base(interp.Python), ShouldEqual, tc.found)
					So(tc.version.IsSatisfiedBy(version), ShouldBeTrue)
				} else {
					// Error case.
					So(err, ShouldErrLike, tc.err)
				}
			})
		}
	})
}
