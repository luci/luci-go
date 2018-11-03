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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestKinds(t *testing.T) {
	t.Parallel()

	Convey("names", t, func() {
		Convey("empty", func() {
			k := &Kinds{}
			So(k.Names(), ShouldBeEmpty)
		})

		Convey("non-empty", func() {
			k := &Kinds{
				Kind: []*Kind{
					{
						Name: "duplicated",
					},
					{},
					{
						Name: "unique",
					},
					{
						Name: "duplicated",
					},
				},
			}
			So(k.Names(), ShouldResemble, []string{"duplicated", "", "unique", "duplicated"})
		})
	})

	Convey("validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("empty", func() {
			k := &Kinds{}
			k.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})

		Convey("names", func() {
			Convey("missing", func() {
				k := &Kinds{
					Kind: []*Kind{
						{},
					},
				}
				k.Validate(c)
				So(c.Finalize(), ShouldErrLike, "name is required")
			})

			Convey("duplicate", func() {
				k := &Kinds{
					Kind: []*Kind{
						{
							Name: "duplicated",
						},
						{
							Name: "unique",
						},
						{
							Name: "duplicated",
						},
					},
				}
				k.Validate(c)
				So(c.Finalize(), ShouldErrLike, "duplicate name")
			})
		})
	})
}
