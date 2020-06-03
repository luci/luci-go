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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestVarSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Works", t, func() {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })
		vs.Register("b", func(context.Context) (string, error) { return "b_val", nil })

		out, err := vs.RenderTemplate(ctx, "${a}")
		So(err, ShouldBeNil)
		So(out, ShouldEqual, "a_val")

		out, err = vs.RenderTemplate(ctx, "${a}${b}")
		So(err, ShouldBeNil)
		So(out, ShouldEqual, "a_valb_val")

		out, err = vs.RenderTemplate(ctx, "${a}_${b}_${a}")
		So(err, ShouldBeNil)
		So(out, ShouldEqual, "a_val_b_val_a_val")
	})

	Convey("Error in the callback", t, func() {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "", fmt.Errorf("boom") })

		_, err := vs.RenderTemplate(ctx, "zzz_${a}")
		So(err, ShouldErrLike, "boom")
	})

	Convey("Missing var", t, func() {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })

		_, err := vs.RenderTemplate(ctx, "zzz_${a}_${zzz}_${a}")
		So(err, ShouldErrLike, `no placeholder named "zzz" is registered`)
	})

	Convey("Double registration", t, func() {
		vs := VarSet{}
		vs.Register("a", func(context.Context) (string, error) { return "a_val", nil })

		So(func() { vs.Register("a", func(context.Context) (string, error) { return "a_val", nil }) }, ShouldPanic)
	})
}
