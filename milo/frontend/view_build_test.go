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

package frontend

import (
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPrepareGetBuildRequest(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestPrepareGetBuildRequest`, t, func(t *ftt.Test) {
		builderID := &buildbucketpb.BuilderID{
			Project: "fake-project",
			Bucket:  "fake-bucket",
			Builder: "fake-builder",
		}

		t.Run("Should parse build number correctly", func(t *ftt.Test) {
			req, err := prepareGetBuildRequest(builderID, "123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req.BuildNumber, should.Equal(123))
			assert.Loosely(t, req.Builder, should.Equal(builderID))
		})

		t.Run("Should reject large build number", func(t *ftt.Test) {
			req, err := prepareGetBuildRequest(builderID, "9223372036854775807")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, req, should.BeNil)
		})

		t.Run("Should reject malformated build number", func(t *ftt.Test) {
			req, err := prepareGetBuildRequest(builderID, "abc")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, req, should.BeNil)
		})

		t.Run("Should parse build ID correctly", func(t *ftt.Test) {
			req, err := prepareGetBuildRequest(builderID, "b123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req.Id, should.Equal(123))
			assert.Loosely(t, req.Builder, should.BeNil)
		})

		t.Run("Should reject large build ID", func(t *ftt.Test) {
			req, err := prepareGetBuildRequest(builderID, "b9223372036854775809")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, req, should.BeNil)
		})
	})
}
