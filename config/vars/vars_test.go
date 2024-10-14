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

package vars

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestVarSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Works", t, func(t *ftt.Test) {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })
		vs.Register("b", func(context.Context) (string, error) { return "b_val", nil })

		out, err := vs.RenderTemplate(ctx, "${a}")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out, should.Equal("a_val"))

		out, err = vs.RenderTemplate(ctx, "${a}${b}")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out, should.Equal("a_valb_val"))

		out, err = vs.RenderTemplate(ctx, "${a}_${b}_${a}")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out, should.Equal("a_val_b_val_a_val"))
	})

	ftt.Run("Error in the callback", t, func(t *ftt.Test) {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "", fmt.Errorf("boom") })

		_, err := vs.RenderTemplate(ctx, "zzz_${a}")
		assert.Loosely(t, err, should.ErrLike("boom"))
	})

	ftt.Run("Missing var", t, func(t *ftt.Test) {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })

		_, err := vs.RenderTemplate(ctx, "zzz_${a}_${zzz}_${a}")
		assert.Loosely(t, err, should.ErrLike(`no placeholder named "zzz" is registered`))
	})

	ftt.Run("Double registration", t, func(t *ftt.Test) {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })

		assert.Loosely(t, func() { vs.Register("a", func(context.Context) (string, error) { return "a_val", nil }) }, should.Panic)
	})
}
