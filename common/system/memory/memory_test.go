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

package memory

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTotalSystemMemoryBytes(t *testing.T) {
	ret, err := TotalSystemMemoryBytes()
	assert.NoErr(t, err)
	t.Log("TotalSystemMemoryBytes:", ret)
	// All systems we run this test on should have > 1GB of memory - this should
	// catch implementations which return a unit less granular than bytes.
	assert.That(t, ret, should.BeGreaterThan[uint64](1024*1024*1024))
}
