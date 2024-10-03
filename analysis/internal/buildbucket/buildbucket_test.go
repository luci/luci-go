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

package buildbucket

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("Get build", t, func(t *ftt.Test) {

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)

		bID := int64(87654321)
		inv := "invocations/build-87654321"

		req := &bbpb.GetBuildRequest{
			Id: bID,
			Mask: &bbpb.BuildMask{
				Fields: &field_mask.FieldMask{
					Paths: []string{"builder", "infra.resultdb", "status"},
				},
			},
		}

		res := &bbpb.Build{
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "builder",
			},
			Infra: &bbpb.BuildInfra{
				Resultdb: &bbpb.BuildInfra_ResultDB{
					Hostname:   "results.api.cr.dev",
					Invocation: inv,
				},
			},
			Status: bbpb.Status_FAILURE,
		}
		reqCopy := proto.Clone(req).(*bbpb.GetBuildRequest)
		mc.GetBuild(reqCopy, res)

		bc, err := NewClient(mc.Ctx, "bbhost", "chromium")
		assert.Loosely(t, err, should.BeNil)
		b, err := bc.GetBuild(mc.Ctx, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.Resemble(res))
	})
}
