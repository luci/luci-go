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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestReadAllForTesting(t *testing.T) {
	ftt.Run("ReadAllForTesting", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run("Reads all rows", func(t *ftt.Test) {
			ri := rootinvocations.NewBuilder("root-inv-id").Build()
			testResult := NewBuilder().Build()

			ms := []*spanner.Mutation{}
			ms = append(ms, rootinvocations.InsertForTesting(ri)...)
			ms = append(ms, InsertForTesting(testResult))
			_, err := span.Apply(ctx, ms)
			assert.Loosely(t, err, should.BeNil)

			// Read all rows
			rows, err := ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)

			// Verify
			assert.Loosely(t, rows, should.HaveLength(1))
			assert.Loosely(t, rows[0], should.Match(testResult))
		})
	})
}
