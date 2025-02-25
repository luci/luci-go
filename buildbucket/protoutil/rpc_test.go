// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestParseGetBuildRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseGetBuildRequest", t, func(t *ftt.Test) {
		t.Run("build ID", func(t *ftt.Test) {
			req, err := ParseGetBuildRequest("1234567890")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Match(&pb.GetBuildRequest{Id: 1234567890}))
		})

		t.Run("build number", func(t *ftt.Test) {
			req, err := ParseGetBuildRequest("chromium/ci/linux-rel/1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Match(&pb.GetBuildRequest{
				Builder: &pb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				BuildNumber: 1,
			}))
		})
	})
}
