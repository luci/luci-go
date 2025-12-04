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

package masking

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
)

func TestRootInvocationEtag(t *testing.T) {
	t.Parallel()
	ftt.Run("TestRootInvocationEtag", t, func(t *ftt.Test) {
		lastUpdatedTime := time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)
		ri := &rootinvocations.RootInvocationRow{LastUpdated: lastUpdatedTime}

		t.Run("RootInvocationEtag", func(t *ftt.Test) {
			etag := RootInvocationEtag(ri)
			assert.That(t, etag, should.Equal(`W/"2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("ParseRootInvocationEtag", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				etag := `W/"2025-04-26T01:02:03.000004Z"`
				lastUpdated, err := ParseRootInvocationEtag(etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, lastUpdated, should.Equal("2025-04-26T01:02:03.000004Z"))
			})
			t.Run("malformed", func(t *ftt.Test) {
				etag := `W/+l/"malformed"`
				_, err := ParseRootInvocationEtag(etag)
				assert.Loosely(t, err, should.ErrLike("malformated etag"))
			})
		})

		t.Run("IsRootInvocationEtagMatch", func(t *ftt.Test) {
			t.Run("round-trip", func(t *ftt.Test) {
				etag := RootInvocationEtag(ri)
				match, err := IsRootInvocationEtagMatch(ri, etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, match, should.BeTrue)
			})
			t.Run("not match", func(t *ftt.Test) {
				etag := RootInvocationEtag(ri)
				ri.LastUpdated = ri.LastUpdated.Add(time.Second)

				match, err := IsRootInvocationEtagMatch(ri, etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, match, should.BeFalse)
			})
		})
	})
}
