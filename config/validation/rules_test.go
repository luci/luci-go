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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRuleSet(t *testing.T) {
	t.Parallel()

	ftt.Run("With validator callback", t, func(t *ftt.Test) {
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

		t.Run("Works", func(t *ftt.Test) {
			r := NewRuleSet()

			r.Vars.Register("a", func(context.Context) (string, error) { return "a_val", nil })
			r.Vars.Register("b", func(context.Context) (string, error) { return "b_val", nil })

			r.Add("services/${a}", "paths/a", validator("rule_1"))
			r.Add("services/${a}", "paths/${b}", validator("rule_2"))
			r.Add("services/c", "paths/c", validator("rule_3"))
			r.Add("regex:services/.*", "paths/c", validator("rule_4"))

			patterns, err := r.ConfigPatterns(ctx)
			assert.Loosely(t, err, should.BeNil)

			type pair struct {
				configSet string
				path      string
			}
			asPairs := make([]pair, len(patterns))
			for i, p := range patterns {
				asPairs[i] = pair{p.ConfigSet.String(), p.Path.String()}
			}
			assert.Loosely(t, asPairs, should.Resemble([]pair{
				{"exact:services/a_val", "exact:paths/a"},
				{"exact:services/a_val", "exact:paths/b_val"},
				{"exact:services/c", "exact:paths/c"},
				{"regex:^services/.*$", "exact:paths/c"},
			}))

			callValidator := func(configSet, path string) {
				c := Context{Context: ctx}
				assert.Loosely(t, r.ValidateConfig(&c, configSet, path, []byte("body")), should.BeNil)
			}

			callValidator("services/unknown", "paths/a")
			callValidator("services/a_val", "paths/a")
			callValidator("services/a_val", "paths/b_val")
			callValidator("services/a_val", "paths/c")
			callValidator("services/c", "paths/c")

			assert.Loosely(t, calls, should.Resemble([]call{
				{"rule_1", "services/a_val", "paths/a"},
				{"rule_2", "services/a_val", "paths/b_val"},
				{"rule_4", "services/a_val", "paths/c"},
				{"rule_3", "services/c", "paths/c"},
				{"rule_4", "services/c", "paths/c"},
			}))
		})

		t.Run("Error in the var callback", func(t *ftt.Test) {
			r := NewRuleSet()
			r.Vars.Register("a", func(context.Context) (string, error) { return "", fmt.Errorf("boom") })
			r.Add("services/${a}", "paths/a", validator("rule_1"))
			err := r.ValidateConfig(&Context{Context: ctx}, "services/zzz", "some path", []byte("body"))
			assert.Loosely(t, err, should.ErrLike("boom"))
		})

		t.Run("Missing variables", func(t *ftt.Test) {
			r := NewRuleSet()
			r.Vars.Register("a", func(context.Context) (string, error) { return "a_val", nil })
			r.Add("${zzz}", "a", validator("1"))
			r.Add("a", "${zzz}", validator("1"))

			_, err := r.ConfigPatterns(ctx)
			merr := err.(errors.MultiError)
			assert.Loosely(t, merr, should.HaveLength(2))
			assert.Loosely(t, merr[0], should.ErrLike(
				`config set pattern "exact:${zzz}": no placeholder named "zzz" is registered`))
			assert.Loosely(t, merr[1], should.ErrLike(
				`path pattern "exact:${zzz}": no placeholder named "zzz" is registered`))

			err = r.ValidateConfig(&Context{Context: ctx}, "set", "path", nil)
			merr = err.(errors.MultiError)
			assert.Loosely(t, merr, should.HaveLength(2))
			assert.Loosely(t, merr[0], should.ErrLike(
				`config set pattern "exact:${zzz}": no placeholder named "zzz" is registered`))
			assert.Loosely(t, merr[1], should.ErrLike(
				`path pattern "exact:${zzz}": no placeholder named "zzz" is registered`))
		})

		t.Run("Pattern is validated", func(t *ftt.Test) {
			r := NewRuleSet()
			r.Vars.Register("a", func(context.Context) (string, error) { return "a_val", nil })
			r.Add("unknown:${a}", "a", validator("1"))
			r.Add("a", "unknown:${a}", validator("1"))

			_, err := r.ConfigPatterns(ctx)
			merr := err.(errors.MultiError)
			assert.Loosely(t, merr, should.HaveLength(2))
			assert.Loosely(t, merr[0], should.ErrLike(
				`config set pattern "unknown:${a}": unknown pattern kind: "unknown"`))
			assert.Loosely(t, merr[1], should.ErrLike(
				`path pattern "unknown:${a}": unknown pattern kind: "unknown"`))

			err = r.ValidateConfig(&Context{Context: ctx}, "set", "path", nil)
			merr = err.(errors.MultiError)
			assert.Loosely(t, merr, should.HaveLength(2))
			assert.Loosely(t, merr[0], should.ErrLike(
				`config set pattern "unknown:${a}": unknown pattern kind: "unknown"`))
			assert.Loosely(t, merr[1], should.ErrLike(
				`path pattern "unknown:${a}": unknown pattern kind: "unknown"`))
		})
	})
}
