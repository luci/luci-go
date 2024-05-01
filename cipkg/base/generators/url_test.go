// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"context"
	"testing"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFetchURLs(t *testing.T) {
	Convey("Test fetch urls", t, func() {
		ctx := context.Background()
		plats := Platforms{}

		Convey("ok", func() {
			g := &FetchURLs{
				Name: "urls",
				URLs: map[string]FetchURL{
					"something1": {
						URL:  "https://host/path1",
						Mode: 0o777,
					},
					"dir1/something2": {
						URL:           "https://host/path2",
						HashAlgorithm: core.HashAlgorithm_HASH_MD5,
						HashValue:     "abcdef",
					},
				},
			}
			a, err := g.Generate(ctx, plats)
			So(err, ShouldBeNil)

			url := testutils.Assert[*core.Action_Copy](t, a.Spec)
			So(url.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
				"something1": {
					Content: &core.ActionFilesCopy_Source_Output_{
						Output: &core.ActionFilesCopy_Source_Output{Name: "urls_2o025r0794", Path: "file"},
					},
					Mode: 0o777,
				},
				"dir1/something2": {
					Content: &core.ActionFilesCopy_Source_Output_{
						Output: &core.ActionFilesCopy_Source_Output{Name: "urls_om04u163h4", Path: "file"},
					},
					Mode: 0o666,
				},
			})

			{
				So(a.Deps, ShouldHaveLength, 2)
				for _, d := range a.Deps {
					u := testutils.Assert[*core.Action_Url](t, d.Spec)
					switch d.Name {
					case "urls_2o025r0794":
						So(u.Url, assertions.ShouldResembleProto, &core.ActionURLFetch{
							Url: "https://host/path1",
						})
					case "urls_om04u163h4":
						So(u.Url, assertions.ShouldResembleProto, &core.ActionURLFetch{
							Url:           "https://host/path2",
							HashAlgorithm: core.HashAlgorithm_HASH_MD5,
							HashValue:     "abcdef",
						})
					}
				}
			}
		})

		// context info should be inherited since urls and url are treated as one
		// thing.
		Convey("context info", func() {
			g := &FetchURLs{
				Name:     "urls",
				Metadata: &core.Action_Metadata{ContextInfo: "info"},
				URLs: map[string]FetchURL{
					"something1": {
						URL:  "https://host/path1",
						Mode: 0o777,
					},
				},
			}
			a, err := g.Generate(ctx, plats)
			So(err, ShouldBeNil)

			So(a.Metadata.GetContextInfo(), ShouldEqual, "info")
			So(a.Deps, ShouldHaveLength, 1)
			So(a.Deps[0].Metadata.GetContextInfo(), ShouldEqual, "info")
		})
	})

}
