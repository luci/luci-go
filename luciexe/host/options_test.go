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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/viewer"
)

func TestOptions(t *testing.T) {
	ftt.Run(`test Options`, t, func(t *ftt.Test) {
		t.Run(`initialize sets logdogTags`, func(t *ftt.Test) {
			opts := Options{}
			builder := &bbpb.BuilderID{
				Builder: "builder-a",
				Bucket:  "bucket-b",
				Project: "proj-c",
			}

			t.Run(`logdog viewer URL`, func(t *ftt.Test) {
				opts.ViewerURL = "https://example.org"

				// w/ BaseBuild
				opts.BaseBuild = &bbpb.Build{Builder: builder}
				assert.Loosely(t, opts.initialize(), should.BeNil)
				assert.Loosely(t, opts.logdogTags, should.Match(streamproto.TagMap{
					"buildbucket.builder":     "builder-a",
					"buildbucket.bucket":      "bucket-b",
					"buildbucket.project":     "proj-c",
					viewer.LogDogViewerURLTag: "https://example.org",
				}))

				// w/o BaseBuild
				opts.BaseBuild = nil
				assert.Loosely(t, opts.initialize(), should.BeNil)
				assert.Loosely(t, opts.logdogTags, should.Match(streamproto.TagMap{
					viewer.LogDogViewerURLTag: "https://example.org",
				}))
			})

			t.Run(`buildbucket`, func(t *ftt.Test) {
				opts.BaseBuild = &bbpb.Build{Builder: builder}

				// w/ ViewerURL
				// (this is done in the above convey

				// w/o ViewerURL
				opts.ViewerURL = ""
				assert.Loosely(t, opts.initialize(), should.BeNil)
				assert.Loosely(t, opts.logdogTags, should.Match(streamproto.TagMap{
					"buildbucket.builder": "builder-a",
					"buildbucket.bucket":  "bucket-b",
					"buildbucket.project": "proj-c",
				}))
			})
		})
	})
}
