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

package gerrit

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGerritTime(t *testing.T) {
	t.Parallel()

	ftt.Run("FormatTime / ParseTime work", t, func(t *ftt.Test) {
		const s = `"2011-02-03 04:05:06.000000007"`
		ts := time.Date(2011, time.February, 3, 4, 5, 6, 7, time.UTC)
		assert.Loosely(t, FormatTime(ts), should.Equal(s))
		pt, err := ParseTime(s)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pt, should.Match(ts.UTC()))
	})
}
