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

package versioning

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	apiv1pb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestStatusV1(t *testing.T) {
	t.Parallel()

	ftt.Run("RunStatusV1 returns a valid enum", t, func(t *ftt.Test) {
		for name, val := range run.Status_value {
			name, val := name, val
			t.Run("for internal."+name, func(t *ftt.Test) {
				eq := RunStatusV1(run.Status(val))

				// check if it's typed with the API enum.
				assert.Loosely(t, eq.Descriptor().FullName(), should.Equal(
					apiv1pb.Run_STATUS_UNSPECIFIED.Descriptor().FullName()))
				// check if it's one of the defined enum ints.
				_, ok := apiv1pb.Run_Status_name[int32(eq)]
				assert.Loosely(t, ok, should.BeTrue)
			})
		}
	})
}

func TestStatusV0(t *testing.T) {
	t.Parallel()

	ftt.Run("RunStatusV0 returns a valid enum", t, func(t *ftt.Test) {
		for name, val := range run.Status_value {
			name, val := name, val
			t.Run("for internal."+name, func(t *ftt.Test) {
				eq := RunStatusV0(run.Status(val))

				// check if it's typed with the API enum.
				assert.Loosely(t, eq.Descriptor().FullName(), should.Equal(
					apiv0pb.Run_STATUS_UNSPECIFIED.Descriptor().FullName()))
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Run_Status_name[int32(eq)]
				assert.Loosely(t, ok, should.BeTrue)
			})
		}
	})

	ftt.Run("TryjobStatusV0 returns a valid enum", t, func(t *ftt.Test) {
		for name, val := range tryjob.Status_value {
			name, val := name, val
			t.Run("for internal."+name, func(t *ftt.Test) {
				eq := LegacyTryjobStatusV0(tryjob.Status(val))

				// check if it's typed with the API enum.
				assert.Loosely(t, eq.Descriptor().FullName(), should.Equal(
					apiv0pb.Tryjob_STATUS_UNSPECIFIED.Descriptor().FullName()))
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Tryjob_Status_name[int32(eq)]
				assert.Loosely(t, ok, should.BeTrue)
			})
		}
	})

	ftt.Run("TryjobResultStatusV0 returns a valid enum", t, func(t *ftt.Test) {
		for name, val := range tryjob.Result_Status_value {
			name, val := name, val
			t.Run("for internal."+name, func(t *ftt.Test) {
				eq := LegacyTryjobResultStatusV0(tryjob.Result_Status(val))

				// check if it's typed with the API enum.
				assert.Loosely(t, eq.Descriptor().FullName(), should.Equal(
					apiv0pb.Tryjob_Result_RESULT_STATUS_UNSPECIFIED.Descriptor().FullName()))
				// check if it's one of the defined enum ints.
				_, ok := apiv0pb.Tryjob_Result_Status_name[int32(eq)]
				assert.Loosely(t, ok, should.BeTrue)
			})
		}
	})
}
