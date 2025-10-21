// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

func TestCalculateFingerprint(t *testing.T) {
	t.Parallel()

	call := func(md *repopb.PrefixMetadata) string {
		CalculateFingerprint(md)
		return md.Fingerprint
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, call(&repopb.PrefixMetadata{}), should.Equal("MvCPQuqSLssfbtMFiqBzZOdINQo"))
		assert.Loosely(t, call(&repopb.PrefixMetadata{Fingerprint: "zzz"}), should.Equal("MvCPQuqSLssfbtMFiqBzZOdINQo"))
		assert.Loosely(t, call(&repopb.PrefixMetadata{Prefix: "a"}), should.Equal("B96o1ElnpJUTe-Hf3dyT_S7KoTw"))
	})
}
