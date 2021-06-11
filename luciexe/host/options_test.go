// Copyright 2021 The LUCI Authors.
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

package host

import (
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/viewer"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOptions(t *testing.T) {
	Convey(`test Options`, t, func() {
		Convey(`initialize sets logdogTags`, func() {
			opts := Options{}
			builder := &bbpb.BuilderID{
				Builder: "builder-a",
				Bucket:  "bucket-b",
				Project: "proj-c",
			}

			Convey(`logdog viewer URL`, func() {
				opts.ViewerURL = "https://example.org"

				// w/ BaseBuild
				opts.BaseBuild = &bbpb.Build{Builder: builder}
				So(opts.initialize(), ShouldBeNil)
				So(opts.logdogTags, ShouldResemble, streamproto.TagMap{
					"buildbucket.builder":     "builder-a",
					"buildbucket.bucket":      "bucket-b",
					"buildbucket.project":     "proj-c",
					viewer.LogDogViewerURLTag: "https://example.org",
				})

				// w/o BaseBuild
				opts.BaseBuild = nil
				So(opts.initialize(), ShouldBeNil)
				So(opts.logdogTags, ShouldResemble, streamproto.TagMap{
					viewer.LogDogViewerURLTag: "https://example.org",
				})
			})

			Convey(`buildbucket`, func() {
				opts.BaseBuild = &bbpb.Build{Builder: builder}

				// w/ ViewerURL
				// (this is done in the above convey

				// w/o ViewerURL
				opts.ViewerURL = ""
				So(opts.initialize(), ShouldBeNil)
				So(opts.logdogTags, ShouldResemble, streamproto.TagMap{
					"buildbucket.builder": "builder-a",
					"buildbucket.bucket":  "bucket-b",
					"buildbucket.project": "proj-c",
				})
			})
		})
	})
}
