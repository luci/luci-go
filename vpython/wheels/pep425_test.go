// Copyright 2022 The LUCI Authors.
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

package wheels

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/api/vpython"
)

func TestPEP425TagSelector(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		tags     []*vpython.PEP425Tag
		template map[string]string
	}{
		{
			[]*vpython.PEP425Tag{
				{Python: "py2", Abi: "none", Platform: "any"},
				{Python: "py27", Abi: "none", Platform: "any"},
				{Python: "cp27", Abi: "cp27mu", Platform: "linux_x86_64"},
				{Python: "cp27", Abi: "cp27mu", Platform: "manylinux1_x86_64"},
				{Python: "cp27", Abi: "none", Platform: "manylinux1_x86_64"},
			},
			map[string]string{
				"platform":         "linux-amd64",
				"py_tag":           "cp27-cp27mu-manylinux1_x86_64",
				"py_python":        "cp27",
				"py_abi":           "cp27mu",
				"py_platform":      "manylinux1_x86_64",
				"vpython_platform": "linux-amd64_cp27_cp27mu",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_12_x86_64"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_12_fat64"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_12_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_12_intel"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_10_intel"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_9_fat64"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_9_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_9_universal"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_8_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_8_universal"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_6_intel"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_6_fat64"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_6_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_6_universal"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_5_universal"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_4_intel"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_4_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_1_universal"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_0_fat32"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_10_0_universal"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_12_x86_64"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_12_intel"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_12_fat64"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_9_universal"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_8_x86_64"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_8_intel"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_7_intel"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_7_fat64"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_7_fat32"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_6_universal"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_5_x86_64"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_5_intel"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_3_fat32"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_3_universal"},
				{Python: "cp27", Abi: "none", Platform: "macosx_10_2_fat32"},
				{Python: "py2", Abi: "none", Platform: "macosx_10_4_intel"},
				{Python: "cp27", Abi: "none", Platform: "any"},
				{Python: "cp27", Abi: "cp27m", Platform: "macosx_11_0_intel"},
				{Python: "py2", Abi: "none", Platform: "macosx_10_3_intel"},
			},
			map[string]string{
				"platform":         "mac-amd64",
				"py_tag":           "cp27-cp27m-macosx_10_4_intel",
				"py_python":        "cp27",
				"py_abi":           "cp27m",
				"py_platform":      "macosx_10_4_intel",
				"vpython_platform": "mac-amd64_cp27_cp27m",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "py27", Abi: "none", Platform: "any"},
				{Python: "py27", Abi: "none", Platform: "linux_i686"},
			},
			map[string]string{
				"platform":         "linux-386",
				"py_tag":           "py27-none-linux_i686",
				"py_python":        "py27",
				"py_abi":           "none",
				"py_platform":      "linux_i686",
				"vpython_platform": "linux-386_py27_none",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "py27", Abi: "none", Platform: "any"},
				{Python: "py27", Abi: "none", Platform: "linux_x86_64"},
			},
			map[string]string{
				"platform":         "linux-amd64",
				"py_tag":           "py27-none-linux_x86_64",
				"py_python":        "py27",
				"py_abi":           "none",
				"py_platform":      "linux_x86_64",
				"vpython_platform": "linux-amd64_py27_none",
			},
		},

		{
			// Tags taken from actual output running vpython3 on Linux x64.
			[]*vpython.PEP425Tag{
				{Python: "cp3", Abi: "none", Platform: "any"},
				{Python: "cp32", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp32", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp33", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp33", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp34", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp34", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp35", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp35", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp36", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp36", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp37", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp37", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp38", Abi: "cp38", Platform: "linux_x86_64"},
				{Python: "cp38", Abi: "cp38", Platform: "manylinux1_x86_64"},
				{Python: "cp38", Abi: "none", Platform: "any"},
				{Python: "cp38", Abi: "none", Platform: "linux_x86_64"},
				{Python: "cp38", Abi: "none", Platform: "manylinux1_x86_64"},
				{Python: "py3", Abi: "none", Platform: "any"},
				{Python: "py3", Abi: "none", Platform: "linux_x86_64"},
				{Python: "py3", Abi: "none", Platform: "manylinux1_x86_64"},
				{Python: "py30", Abi: "none", Platform: "any"},
				{Python: "py31", Abi: "none", Platform: "any"},
				{Python: "py32", Abi: "none", Platform: "any"},
				{Python: "py33", Abi: "none", Platform: "any"},
				{Python: "py34", Abi: "none", Platform: "any"},
				{Python: "py35", Abi: "none", Platform: "any"},
				{Python: "py36", Abi: "none", Platform: "any"},
				{Python: "py37", Abi: "none", Platform: "any"},
				{Python: "py38", Abi: "none", Platform: "any"},
			},
			map[string]string{
				"platform":         "linux-amd64",
				"py_tag":           "cp38-cp38-manylinux1_x86_64",
				"py_python":        "cp38",
				"py_abi":           "cp38",
				"py_platform":      "manylinux1_x86_64",
				"vpython_platform": "linux-amd64_cp38_cp38",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "cp3", Abi: "none", Platform: "any"},
				{Python: "cp32", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp32", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp33", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp33", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp34", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp34", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp35", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp35", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp36", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp36", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp37", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp37", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "linux_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "manylinux1_x86_64"},
				{Python: "cp39", Abi: "cp39", Platform: "linux_x86_64"},
				{Python: "cp39", Abi: "cp39", Platform: "manylinux1_x86_64"},
				{Python: "cp39", Abi: "none", Platform: "any"},
				{Python: "cp39", Abi: "none", Platform: "linux_x86_64"},
				{Python: "cp39", Abi: "none", Platform: "manylinux1_x86_64"},
				{Python: "py3", Abi: "none", Platform: "any"},
				{Python: "py3", Abi: "none", Platform: "linux_x86_64"},
				{Python: "py3", Abi: "none", Platform: "manylinux1_x86_64"},
				{Python: "py30", Abi: "none", Platform: "any"},
				{Python: "py31", Abi: "none", Platform: "any"},
				{Python: "py32", Abi: "none", Platform: "any"},
				{Python: "py33", Abi: "none", Platform: "any"},
				{Python: "py34", Abi: "none", Platform: "any"},
				{Python: "py35", Abi: "none", Platform: "any"},
				{Python: "py36", Abi: "none", Platform: "any"},
				{Python: "py37", Abi: "none", Platform: "any"},
				{Python: "py38", Abi: "none", Platform: "any"},
			},
			map[string]string{
				"platform":         "linux-amd64",
				"py_tag":           "cp39-cp39-manylinux1_x86_64",
				"py_python":        "cp39",
				"py_abi":           "cp39",
				"py_platform":      "manylinux1_x86_64",
				"vpython_platform": "linux-amd64_cp39_cp39",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "cp38", Abi: "cp38", Platform: "win_amd64"},
				{Python: "cp38", Abi: "none", Platform: "win_amd64"},
				{Python: "py3", Abi: "none", Platform: "win_amd64"},
				{Python: "cp38", Abi: "none", Platform: "any"},
				{Python: "cp3", Abi: "none", Platform: "any"},
				{Python: "py38", Abi: "none", Platform: "any"},
				{Python: "py3", Abi: "none", Platform: "any"},
				{Python: "py37", Abi: "none", Platform: "any"},
				{Python: "py36", Abi: "none", Platform: "any"},
				{Python: "py35", Abi: "none", Platform: "any"},
				{Python: "py34", Abi: "none", Platform: "any"},
				{Python: "py33", Abi: "none", Platform: "any"},
				{Python: "py32", Abi: "none", Platform: "any"},
				{Python: "py31", Abi: "none", Platform: "any"},
				{Python: "py30", Abi: "none", Platform: "any"},
			},
			map[string]string{
				"platform":         "windows-amd64",
				"py_tag":           "cp38-cp38-win_amd64",
				"py_python":        "cp38",
				"py_abi":           "cp38",
				"py_platform":      "win_amd64",
				"vpython_platform": "windows-amd64_cp38_cp38",
			},
		},

		{
			[]*vpython.PEP425Tag{
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_x86_64"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_intel"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_fat64"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_fat32"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_arm64"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_12_universal"},
				{Python: "cp38", Abi: "abi3", Platform: "macosx_10_11_arm64"},
			},
			map[string]string{
				"platform":         "mac-arm64",
				"py_tag":           "cp38-abi3-macosx_10_11_arm64",
				"py_python":        "cp38",
				"py_abi":           "abi3",
				"py_platform":      "macosx_10_11_arm64",
				"vpython_platform": "mac-arm64_cp38_abi3",
			},
		},
	}

	ftt.Run(`Testing PEP425 tag selection`, t, func(t *ftt.Test) {
		for i, tc := range testCases {
			t.Run(fmt.Sprintf(`case #%d`, i), func(t *ftt.Test) {
				tagsStr := make([]string, len(tc.tags))
				for i, tag := range tc.tags {
					tagsStr[i] = tag.TagString()
				}

				t.Log("TagsStr", tagsStr)
				tag := pep425TagSelector(tc.tags)

				expander := make(template.Expander)
				err := addPEP425CIPDTemplateForTag(expander, tag)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, map[string]string(expander), should.Resemble(tc.template))
			})
		}

		t.Run(`Returns an error when no tag is selected.`, func(t *ftt.Test) {
			tag := pep425TagSelector(nil)
			assert.Loosely(t, tag, should.BeNil)

			expander := make(template.Expander)
			err := addPEP425CIPDTemplateForTag(expander, tag)
			assert.Loosely(t, err, should.ErrLike("no PEP425 tag"))
		})

		t.Run(`Returns an error when an unknown platform is selected.`, func(t *ftt.Test) {
			tag := pep425TagSelector([]*vpython.PEP425Tag{
				{Python: "py27", Abi: "none", Platform: "any"},
				{Python: "py27", Abi: "foo", Platform: "bar"},
			})
			assert.Loosely(t, tag, should.Resemble(&vpython.PEP425Tag{Python: "py27", Abi: "foo", Platform: "bar"}))

			expander := make(template.Expander)
			err := addPEP425CIPDTemplateForTag(expander, tag)
			assert.Loosely(t, err, should.ErrLike("failed to infer CIPD platform for tag"))
		})
	})
}
