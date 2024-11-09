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

package loggingutil

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/bisection/util/testutil"
)

func TestLogging(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("Logging", t, func(t *ftt.Test) {
		testutil.CreateCompileFailureAnalysisAnalysisChain(c, t, 123, "chromium", 456)
		c, err := UpdateLoggingWithAnalysisID(c, 456)
		assert.Loosely(t, err, should.BeNil)

		// Check the logging fields
		entries := logging.GetFields(c).SortedEntries()
		assert.Loosely(t, checkEntries(entries, "analyzed_bbid", int64(123)), should.BeTrue)
		assert.Loosely(t, checkEntries(entries, "analysis_id", int64(456)), should.BeTrue)
	})
}

func checkEntries(entries []*logging.FieldEntry, k string, v any) bool {
	for _, entry := range entries {
		if entry.Key == k && entry.Value == v {
			return true
		}
	}
	return false
}
