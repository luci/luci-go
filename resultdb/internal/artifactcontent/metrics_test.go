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

package artifactcontent

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMetrics(t *testing.T) {
	ftt.Run("sizeBucket", t, func(t *ftt.Test) {
		assert.Loosely(t, sizeBucket(-1), should.Equal(firstBucket))
		assert.Loosely(t, sizeBucket(0), should.Equal(firstBucket))
		assert.Loosely(t, sizeBucket(1000), should.BeLessThan(sizeBucket(1000*1000)))
		assert.Loosely(t, sizeBucket(0x7FFFFFFFFFFFFFFF), should.BeGreaterThan(0))
	})
}
