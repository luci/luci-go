// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateFilter(t *testing.T) {
	ftt.Run("ValidateFilter", t, func(t *ftt.Test) {
		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Unspecified", func(t *ftt.Test) {
				err := ValidateFilter([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_UNSPECIFIED,
				})
				assert.Loosely(t, err, should.ErrLike("must not contain VERDICT_EFFECTIVE_STATUS_UNSPECIFIED"))
			})
			t.Run("Duplicate", func(t *ftt.Test) {
				err := ValidateFilter([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
				})
				assert.Loosely(t, err, should.ErrLike("must not contain duplicates (VERDICT_EFFECTIVE_STATUS_FAILED appears twice)"))
			})
			t.Run("Out of range", func(t *ftt.Test) {
				err := ValidateFilter([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
					pb.VerdictEffectiveStatus(51),
				})
				assert.Loosely(t, err, should.ErrLike("contains unknown verdict effective status 51"))
			})
		})
		t.Run("Valid", func(t *ftt.Test) {
			err := ValidateFilter([]pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestWhereClause(t *testing.T) {
	ftt.Run("whereClause", t, func(t *ftt.Test) {
		params := make(map[string]any)

		t.Run("Empty", func(t *ftt.Test) {
			got, err := whereClause(nil, params)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Equal("TRUE"))
		})

		t.Run("Exonerated", func(t *ftt.Test) {
			got, err := whereClause([]pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
			}, params)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Equal("(StatusOverride = 2)"))
		})

		t.Run("Failed", func(t *ftt.Test) {
			got, err := whereClause([]pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
			}, params)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Equal("(StatusOverride = 1 AND Status IN UNNEST(@effectiveStatusFilterStatuses))"))
			assert.Loosely(t, params["effectiveStatusFilterStatuses"], should.Match([]int64{int64(pb.TestVerdict_FAILED)}))
		})

		t.Run("Mixed", func(t *ftt.Test) {
			got, err := whereClause([]pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY,
			}, params)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Equal("((StatusOverride = 2) OR (StatusOverride = 1 AND Status IN UNNEST(@effectiveStatusFilterStatuses)))"))
			assert.Loosely(t, params["effectiveStatusFilterStatuses"], should.Match([]int64{
				int64(pb.TestVerdict_FAILED),
				int64(pb.TestVerdict_FLAKY),
			}))
		})
	})
}
