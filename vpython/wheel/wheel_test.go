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

package wheel

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/testfs"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	wheelMarkupSafe = Name{
		Distribution: "MarkupSafe",
		Version:      "0.23",
		BuildTag:     "0",
		PythonTag:    "cp27",
		ABITag:       "none",
		PlatformTag:  "macosx_10_9_intel",
	}
	wheelSimpleJSON = Name{
		Distribution: "simplejson",
		Version:      "3.6.5",
		BuildTag:     "1337",
		PythonTag:    "cp27",
		ABITag:       "none",
		PlatformTag:  "none",
	}
	wheelCryptography = Name{
		Distribution: "cryptography",
		Version:      "1.4",
		BuildTag:     "1",
		PythonTag:    "cp27",
		ABITag:       "cp27m",
		PlatformTag:  "macosx_10_10_intel",
	}
)

func TestName(t *testing.T) {
	t.Parallel()

	var successes = []struct {
		v   string
		exp Name
	}{
		{"MarkupSafe-0.23-0-cp27-none-macosx_10_9_intel.whl", Name{
			Distribution: "MarkupSafe",
			Version:      "0.23",
			BuildTag:     "0",
			PythonTag:    "cp27",
			ABITag:       "none",
			PlatformTag:  "macosx_10_9_intel",
		}},
		{"cryptography-1.4-1-cp27-cp27m-macosx_10_10_intel.whl", Name{
			Distribution: "cryptography",
			Version:      "1.4",
			BuildTag:     "1",
			PythonTag:    "cp27",
			ABITag:       "cp27m",
			PlatformTag:  "macosx_10_10_intel",
		}},
		{"numpy-1.11.0-0_b6a34c03e3a3cea974e4c0000788d4edc7d43a36-cp27-cp27m-" +
			"macosx_10_6_intel.macosx_10_9_intel.macosx_10_9_x86_64.macosx_10_10_intel.macosx_10_10_x86_64.whl",
			Name{
				Distribution: "numpy",
				Version:      "1.11.0",
				BuildTag:     "0_b6a34c03e3a3cea974e4c0000788d4edc7d43a36",
				PythonTag:    "cp27",
				ABITag:       "cp27m",
				PlatformTag:  "macosx_10_6_intel.macosx_10_9_intel.macosx_10_9_x86_64.macosx_10_10_intel.macosx_10_10_x86_64",
			}},
		{"simplejson-3.6.5-1337-cp27-none-none.whl", Name{
			Distribution: "simplejson",
			Version:      "3.6.5",
			BuildTag:     "1337",
			PythonTag:    "cp27",
			ABITag:       "none",
			PlatformTag:  "none",
		}},
		{"nobuildtag-1.2.3-cp27-none-none.whl", Name{
			Distribution: "nobuildtag",
			Version:      "1.2.3",
			BuildTag:     "",
			PythonTag:    "cp27",
			ABITag:       "none",
			PlatformTag:  "none",
		}},
	}

	var failures = []struct {
		v   string
		err string
	}{
		{"foo-bar-baz-qux-quux", "missing .whl suffix"},
		{"foo-bar-baz-qux.whl", "unknown number of segments"},
	}

	Convey(`Testing wheel name parsing`, t, func() {
		for _, tc := range successes {
			Convey(fmt.Sprintf(`Success: %s`, tc.v), func() {
				wn, err := ParseName(tc.v)
				So(err, ShouldBeNil)
				So(wn, ShouldResemble, tc.exp)
				So(wn.String(), ShouldEqual, tc.v)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf(`Failure: %s`, tc.v), func() {
				_, err := ParseName(tc.v)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})
}

func TestScanDir(t *testing.T) {
	t.Parallel()

	Convey(`Testing ScanDir`, t, func() {
		tdir := t.TempDir()
		mustBuild := func(layout map[string]string) {
			if err := testfs.Build(tdir, layout); err != nil {
				panic(err)
			}
		}

		mustBuild(map[string]string{
			"junk.bin":                       "",
			"junk":                           "",
			wheelMarkupSafe.String():         "",
			wheelSimpleJSON.String():         "",
			wheelCryptography.String() + "/": "", // Directories should be ignored.
		})

		Convey(`With no malformed wheels, picks up wheel names.`, func() {
			wheels, err := ScanDir(tdir)
			So(err, ShouldBeNil)
			So(wheels, ShouldResemble, []Name{wheelMarkupSafe, wheelSimpleJSON})
		})

		Convey(`With a malformed wheel name, fails.`, func() {
			mustBuild(map[string]string{
				"malformed-thing.whl": "",
			})

			_, err := ScanDir(tdir)
			So(err, ShouldErrLike, "failed to parse wheel")
		})
	})
}

func TestWriteRequirementsFile(t *testing.T) {
	t.Parallel()

	Convey(`Can write a requirements file.`, t, func() {
		tdir := t.TempDir()
		similarSimpleJSON := wheelSimpleJSON
		similarSimpleJSON.ABITag = "some_other_abi"

		req := filepath.Join(tdir, "requirements.txt")
		err := WriteRequirementsFile(req, []Name{
			wheelMarkupSafe,
			wheelSimpleJSON,
			similarSimpleJSON,
			wheelCryptography})
		So(err, ShouldBeNil)

		content, err := ioutil.ReadFile(req)
		So(err, ShouldBeNil)
		So(content, ShouldResemble, []byte(""+
			"MarkupSafe==0.23\n"+
			"simplejson==3.6.5\n"+
			"cryptography==1.4\n"))
	})
}
