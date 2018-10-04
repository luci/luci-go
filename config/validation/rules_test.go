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

package validation

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRuleSet(t *testing.T) {
	t.Parallel()

	Convey("With validator callback", t, func() {
		ctx := context.Background()

		type call struct {
			validatorID string
			configSet   string
			path        string
		}
		calls := []call{}

		validator := func(id string) Func {
			return func(ctx *Context, configSet, path string, content []byte) error {
				calls = append(calls, call{id, configSet, path})
				return nil
			}
		}

		Convey("Works", func() {
			r := RuleSet{}

			r.RegisterVar("a", func(context.Context) string { return "a_val" })
			r.RegisterVar("b", func(context.Context) string { return "b_val" })

			r.Add("services/${a}", "paths/a", validator("rule_1"))
			r.Add("services/${a}", "paths/${b}", validator("rule_2"))
			r.Add("services/c", "paths/c", validator("rule_3"))
			r.Add("regex:services/.*", "paths/c", validator("rule_4"))

			patterns, err := r.ConfigPatterns(ctx)
			So(err, ShouldBeNil)

			type pair struct {
				configSet string
				path      string
			}
			asPairs := make([]pair, len(patterns))
			for i, p := range patterns {
				asPairs[i] = pair{p.ConfigSet.String(), p.Path.String()}
			}
			So(asPairs, ShouldResemble, []pair{
				{"exact:services/a_val", "exact:paths/a"},
				{"exact:services/a_val", "exact:paths/b_val"},
				{"exact:services/c", "exact:paths/c"},
				{"regex:^services/.*$", "exact:paths/c"},
			})

			callValidator := func(configSet, path string) {
				c := Context{Context: ctx}
				So(r.ValidateConfig(&c, configSet, path, []byte("body")), ShouldBeNil)
			}

			callValidator("services/unknown", "paths/a")
			callValidator("services/a_val", "paths/a")
			callValidator("services/a_val", "paths/b_val")
			callValidator("services/a_val", "paths/c")
			callValidator("services/c", "paths/c")

			So(calls, ShouldResemble, []call{
				{"rule_1", "services/a_val", "paths/a"},
				{"rule_2", "services/a_val", "paths/b_val"},
				{"rule_4", "services/a_val", "paths/c"},
				{"rule_3", "services/c", "paths/c"},
				{"rule_4", "services/c", "paths/c"},
			})
		})

		Convey("Missing variables", func() {
			r := RuleSet{}
			r.RegisterVar("a", func(context.Context) string { return "a_val" })
			So(func() { r.Add("${zzz}", "a", validator("1")) }, ShouldPanicWith,
				`bad config set pattern "exact:${zzz}" - no placeholder named "zzz" is registered`)
			So(func() { r.Add("a", "${zzz}", validator("1")) }, ShouldPanicWith,
				`bad path pattern "exact:${zzz}" - no placeholder named "zzz" is registered`)
		})

		Convey("Pattern is validated", func() {
			r := RuleSet{}
			r.RegisterVar("a", func(context.Context) string { return "a_val" })
			So(func() { r.Add("unknown:${a}", "a", validator("1")) }, ShouldPanicWith,
				`bad config set pattern "unknown:${a}" - unknown pattern kind: "unknown"`)
			So(func() { r.Add("a", "unknown:${a}", validator("1")) }, ShouldPanicWith,
				`bad path pattern "unknown:${a}" - unknown pattern kind: "unknown"`)
		})

		Convey("Duplicated variable", func() {
			r := RuleSet{}
			r.RegisterVar("a", func(context.Context) string { return "a_val" })
			So(func() { r.RegisterVar("a", func(context.Context) string { return "a_val" }) }, ShouldPanic)
		})
	})
}
