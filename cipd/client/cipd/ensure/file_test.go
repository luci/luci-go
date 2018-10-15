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

package ensure

import (
	"bytes"
	"testing"

	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/template"

	. "github.com/smartystreets/goconvey/convey"
)

func mustMakePlatform(v string) template.Platform {
	plat, err := template.ParsePlatform(v)
	if err != nil {
		panic(err)
	}
	return plat
}

var fileSerializationTests = []struct {
	name   string
	f      *File
	expect string
}{
	{
		"empty",
		&File{},
		"",
	},

	{
		"ServiceURL",
		&File{"https://something.example.com", "", "", nil, nil},
		f(
			"$ServiceURL https://something.example.com",
		),
	},

	{
		"simple packages",
		&File{"", "", "", map[string]PackageSlice{
			"": {
				PackageDef{"some/thing", "version", 0},
				PackageDef{"some/other_thing", "latest", 0},
			},
		}, nil},
		f(
			"some/other_thing  latest",
			"some/thing        version",
		),
	},

	{
		"full file",
		&File{
			ServiceURL:       "https://some.example.com",
			ParanoidMode:     deployer.CheckPresence,
			ResolvedVersions: "resolved.versions",
			PackagesBySubdir: map[string]PackageSlice{
				"": {
					PackageDef{"some/thing", "version", 0},
					PackageDef{"some/other_thing", "latest", 0},
				},
				"path/to dir/with/spaces": {
					PackageDef{"different/package", "some_tag:thingy", 0},
				},
			},
			VerifyPlatforms: []template.Platform{
				mustMakePlatform("zoops-ohai"),
				mustMakePlatform("foos-barch"),
			},
		},
		f(
			"$ServiceURL https://some.example.com",
			"$ParanoidMode CheckPresence",
			"$ResolvedVersions resolved.versions",
			"",
			"$VerifiedPlatform zoops-ohai",
			"$VerifiedPlatform foos-barch",
			"",
			"some/other_thing  latest",
			"some/thing        version",
			"",
			"@Subdir path/to dir/with/spaces",
			"different/package  some_tag:thingy",
		),
	},
}

func TestFileSerialization(t *testing.T) {
	t.Parallel()

	Convey("File.Serialize", t, func() {
		for _, tc := range fileSerializationTests {
			Convey(tc.name, func() {
				buf := &bytes.Buffer{}
				So(tc.f.Serialize(buf), ShouldBeNil)
				So(buf.String(), ShouldEqual, tc.expect)
			})
		}
	})
}
