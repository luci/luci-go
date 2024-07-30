// Copyright 2024 The LUCI Authors.
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

package testoutputs

import (
	"go.chromium.org/luci/common/testing/ftt"
	"testing"
)

func TestReset(t *testing.T) {
	t.Parallel()

	state := 0

	ftt.Run("something", t, func(c *ftt.Test) {
		c.Cleanup(func() { state += 1 })
	})

	ftt.Run("else", t, func(c *ftt.Test) {
		c.Run("inner", func(c *ftt.Test) {
			c.Cleanup(func() { state += 1 })
		})
	})

	if state != 2 {
		t.Fatalf("state had wrong value %d != 2", state)
	}
}
