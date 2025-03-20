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

package internal

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSyncWriteMap(t *testing.T) {
	t.Parallel()

	var sm SyncWriteMap[string, int]
	sm.Put("one", 1)
	sm.Put("two", 2)

	assert.That(t, sm.Get("one"), should.Equal(1))
	assert.That(t, sm.Get("two"), should.Equal(2))
	assert.That(t, sm.Get("missing"), should.Equal(0))
}
