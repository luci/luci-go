// Copyright 2025 The LUCI Authors.
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

package recorder

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateBatchFinalizeWorkUnitsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateBatchFinalizeWorkUnitsRequest", t, func(t *ftt.Test) {
		req := &pb.BatchFinalizeWorkUnitsRequest{
			Requests: []*pb.FinalizeWorkUnitRequest{
				{
					Name: "rootInvocations/u-my-root-id/workUnits/u-wu1",
				},
				{
					Name: "rootInvocations/u-my-root-id/workUnits/u-wu2",
				},
			},
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateBatchFinalizeWorkUnitsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no requests", func(t *ftt.Test) {
			req.Requests = nil
			err := validateBatchFinalizeWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})

		t.Run("too many requests", func(t *ftt.Test) {
			req.Requests = make([]*pb.FinalizeWorkUnitRequest, 501)
			err := validateBatchFinalizeWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})
	})
}
