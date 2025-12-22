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

package testresultsv2

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWhereClause(t *testing.T) {
	ftt.Run("WhereClause", t, func(t *ftt.Test) {
		t.Run("Empty filter", func(t *ftt.Test) {
			result, _, err := WhereClause("", "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("(TRUE)"))
		})
		t.Run("Test ID", func(t *ftt.Test) {
			filter := `test_id=":module!scheme:coarse_name:fine_name#case_name"`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`(ModuleName = @tr0 AND ModuleScheme = @tr1 AND T1CoarseName = @tr2 AND T2FineName = @tr3 AND T3CaseName = @tr4)`))
		})
		t.Run("Test ID structured", func(t *ftt.Test) {
			filter := `test_id_structured.module_name="modulename"` +
				` AND test_id_structured.module_scheme="modulescheme"` +
				` AND test_id_structured.module_variant.key="variantvalue"` +
				` AND test_id_structured.module_variant_hash="modulevarianthash"` +
				` AND test_id_structured.coarse_name="coarsename"` +
				` AND test_id_structured.fine_name="finename"` +
				` AND test_id_structured.case_name="casename"`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`((T.ModuleName = @tr0) AND (T.ModuleScheme = @tr1)`+
				` AND (@tr2 IN UNNEST(T.ModuleVariantMasked)) AND (T.ModuleVariantHash = @tr3)`+
				` AND (T.T1CoarseName = @tr4) AND (T.T2FineName = @tr5) AND (T.T3CaseName = @tr6))`))
		})
		t.Run("Tags", func(t *ftt.Test) {
			filter := `tags.key="tagvalue"`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`(@tr0 IN UNNEST(T.TagsMasked))`))
		})
		t.Run("Test Metadata", func(t *ftt.Test) {
			filter := `test_metadata.name="testmetadataname"` +
				` AND test_metadata.location.repo="testmetadatalocationrepo"` +
				` AND test_metadata.location.file_name="testmetadatalocationfilename"`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`((T.TestMetadataNameMasked = @tr0) AND (T.TestMetadataLocationRepoMasked = @tr1) AND (T.TestMetadataLocationFileNameMasked = @tr2))`))
		})
		t.Run("Duration", func(t *ftt.Test) {
			filter := `duration>100s`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`(T.RunDurationNanos > 100000000000)`))
		})
		t.Run("Status", func(t *ftt.Test) {
			filter := `status=EXECUTION_ERRORED`
			result, _, err := WhereClause(filter, "T", "tr")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`(T.StatusV2 = 4)`))
		})
	})
}
