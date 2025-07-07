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

func TestValidateFinalizeWorkUnitRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateFinalizeWorkUnitRequest", t, func(t *ftt.Test) {
		req := &pb.FinalizeWorkUnitRequest{
			Name: "rootInvocations/u-my-root-id/workUnits/u-my-work-unit-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateFinalizeWorkUnitRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("name", func(t *ftt.Test) {
			// Just check we are delegating to the correct validation method,
			// it has its own test cases already.
			t.Run("empty", func(t *ftt.Test) {
				req.Name = ""
				err := validateFinalizeWorkUnitRequest(req)
				assert.Loosely(t, err, should.ErrLike("name: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.Name = "invalid"
				err := validateFinalizeWorkUnitRequest(req)
				assert.Loosely(t, err, should.ErrLike("name: does not match"))
			})
		})
	})
}
