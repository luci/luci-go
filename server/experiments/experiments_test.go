// Copyright 2020 The LUCI Authors.
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

package experiments

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	exp1 = Register("exp1")
	exp2 = Register("exp2")
)

func TestExperiments(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		e1, ok := GetByName("exp1")
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, e1, should.Resemble(exp1))
		assert.Loosely(t, e1.Valid(), should.BeTrue)

		bad, ok := GetByName("bad")
		assert.Loosely(t, ok, should.BeFalse)
		assert.Loosely(t, bad.Valid(), should.BeFalse)

		ctx := context.Background()
		assert.Loosely(t, exp1.Enabled(ctx), should.BeFalse)
		assert.Loosely(t, exp2.Enabled(ctx), should.BeFalse)

		ctx = Enable(ctx) // noop
		assert.Loosely(t, exp1.Enabled(ctx), should.BeFalse)
		assert.Loosely(t, exp2.Enabled(ctx), should.BeFalse)

		ctx = Enable(ctx, exp1)
		assert.Loosely(t, exp1.Enabled(ctx), should.BeTrue)
		assert.Loosely(t, exp2.Enabled(ctx), should.BeFalse)

		ctx = Enable(ctx, exp1) // noop
		assert.Loosely(t, exp1.Enabled(ctx), should.BeTrue)
		assert.Loosely(t, exp2.Enabled(ctx), should.BeFalse)

		ctx2 := Enable(ctx, exp2)
		assert.Loosely(t, exp1.Enabled(ctx2), should.BeTrue)
		assert.Loosely(t, exp2.Enabled(ctx2), should.BeTrue)

		// Hasn't touched the existing context.
		assert.Loosely(t, exp2.Enabled(ctx), should.BeFalse)
	})
}
