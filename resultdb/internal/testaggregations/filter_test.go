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

package testaggregations

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateFilter(t *testing.T) {
	ftt.Run("ValidateFilter", t, func(t *ftt.Test) {
		t.Run("ValidateVerdictStatusFilter", func(t *ftt.Test) {
			t.Run("Invalid", func(t *ftt.Test) {
				t.Run("Unspecified", func(t *ftt.Test) {
					err := ValidateVerdictStatusFilter([]pb.VerdictEffectiveStatus{
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_UNSPECIFIED,
					})
					assert.Loosely(t, err, should.ErrLike("must not contain VERDICT_EFFECTIVE_STATUS_UNSPECIFIED"))
				})
				t.Run("Duplicate", func(t *ftt.Test) {
					err := ValidateVerdictStatusFilter([]pb.VerdictEffectiveStatus{
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
					})
					assert.Loosely(t, err, should.ErrLike("must not contain duplicates (VERDICT_EFFECTIVE_STATUS_FAILED appears twice)"))
				})
				t.Run("Out of range", func(t *ftt.Test) {
					err := ValidateVerdictStatusFilter([]pb.VerdictEffectiveStatus{
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
						pb.VerdictEffectiveStatus(51),
					})
					assert.Loosely(t, err, should.ErrLike("contains unknown verdict effective status 51"))
				})
			})
			t.Run("Valid", func(t *ftt.Test) {
				err := ValidateVerdictStatusFilter([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("ValidateModuleStatusFilter", func(t *ftt.Test) {
			t.Run("Invalid", func(t *ftt.Test) {
				t.Run("Duplicate", func(t *ftt.Test) {
					err := ValidateModuleStatusFilter([]pb.TestAggregation_ModuleStatus{
						pb.TestAggregation_FAILED,
						pb.TestAggregation_FAILED,
					})
					assert.Loosely(t, err, should.ErrLike("must not contain duplicates (FAILED appears twice)"))
				})
				t.Run("Out of range", func(t *ftt.Test) {
					err := ValidateModuleStatusFilter([]pb.TestAggregation_ModuleStatus{
						pb.TestAggregation_FAILED,
						pb.TestAggregation_ModuleStatus(51),
					})
					assert.Loosely(t, err, should.ErrLike("contains unknown module status 51"))
				})
			})
			t.Run("Valid", func(t *ftt.Test) {
				err := ValidateModuleStatusFilter([]pb.TestAggregation_ModuleStatus{
					pb.TestAggregation_FAILED,
					pb.TestAggregation_SUCCEEDED,
					pb.TestAggregation_MODULE_STATUS_UNSPECIFIED,
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestWhereClause(t *testing.T) {
	ftt.Run("whereClause", t, func(t *ftt.Test) {
		params := make(map[string]any)

		t.Run("whereVerdictStatusClause", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				got, err := whereVerdictStatusClause(nil, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("FALSE"))
			})

			t.Run("Exonerated", func(t *ftt.Test) {
				got, err := whereVerdictStatusClause([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("(IsExonerated AND VerdictStatus IN UNNEST(@vsfAllExonerableStatuses))"))
				assert.Loosely(t, params["vsfAllExonerableStatuses"], should.Match([]int64{
					int64(pb.TestVerdict_FAILED),
					int64(pb.TestVerdict_EXECUTION_ERRORED),
					int64(pb.TestVerdict_PRECLUDED),
					int64(pb.TestVerdict_FLAKY),
				}))
			})

			t.Run("Failed", func(t *ftt.Test) {
				got, err := whereVerdictStatusClause([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("(NOT IsExonerated AND VerdictStatus IN UNNEST(@vsfExonerableStatuses))"))
				assert.Loosely(t, params["vsfExonerableStatuses"], should.Match([]int64{int64(pb.TestVerdict_FAILED)}))
			})

			t.Run("Passed", func(t *ftt.Test) {
				got, err := whereVerdictStatusClause([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("(VerdictStatus IN UNNEST(@vsfPassedOrSkippedStatuses))"))
				assert.Loosely(t, params["vsfPassedOrSkippedStatuses"], should.Match([]int64{int64(pb.TestVerdict_PASSED)}))
			})

			t.Run("Mixed", func(t *ftt.Test) {
				got, err := whereVerdictStatusClause([]pb.VerdictEffectiveStatus{
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
					pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("((IsExonerated AND VerdictStatus IN UNNEST(@vsfAllExonerableStatuses)) OR (VerdictStatus IN UNNEST(@vsfPassedOrSkippedStatuses)) OR (NOT IsExonerated AND VerdictStatus IN UNNEST(@vsfExonerableStatuses)))"))
				assert.Loosely(t, params["vsfAllExonerableStatuses"], should.Match([]int64{
					int64(pb.TestVerdict_FAILED),
					int64(pb.TestVerdict_EXECUTION_ERRORED),
					int64(pb.TestVerdict_PRECLUDED),
					int64(pb.TestVerdict_FLAKY),
				}))
				assert.Loosely(t, params["vsfPassedOrSkippedStatuses"], should.Match([]int64{int64(pb.TestVerdict_PASSED)}))
				assert.Loosely(t, params["vsfExonerableStatuses"], should.Match([]int64{int64(pb.TestVerdict_FAILED)}))
			})
		})

		t.Run("whereModuleStatusFilter", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				got, err := whereModuleStatusFilter(nil, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("FALSE"))
			})

			t.Run("Failed", func(t *ftt.Test) {
				got, err := whereModuleStatusFilter([]pb.TestAggregation_ModuleStatus{
					pb.TestAggregation_FAILED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("(ModuleStatus IN UNNEST(@msfStatuses))"))
				assert.Loosely(t, params["msfStatuses"], should.Match([]int64{int64(pb.TestAggregation_FAILED)}))
			})

			t.Run("Mixed", func(t *ftt.Test) {
				got, err := whereModuleStatusFilter([]pb.TestAggregation_ModuleStatus{
					pb.TestAggregation_FAILED,
					pb.TestAggregation_SUCCEEDED,
				}, params)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Equal("(ModuleStatus IN UNNEST(@msfStatuses))"))
				assert.Loosely(t, params["msfStatuses"], should.Match([]int64{
					int64(pb.TestAggregation_FAILED),
					int64(pb.TestAggregation_SUCCEEDED),
				}))
			})
		})
	})
}
