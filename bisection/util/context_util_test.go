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

package util

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSetLUCIProject(t *testing.T) {
	ftt.Run("WithLUCIProject", t, func(t *ftt.Test) {
		c := context.Background()
		c = WithLUCIProject(c, "testproject")
		assert.Loosely(t, LUCIProject(c), should.Equal("testproject"))
	})

	ftt.Run("WithoutLUCIProject", t, func(t *ftt.Test) {
		c := context.Background()
		assert.Loosely(t, LUCIProject(c), should.Equal("chromium"))
	})
}
