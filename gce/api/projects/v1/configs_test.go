// Copyright 2019 The LUCI Authors.
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

package projects

import (
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				cfgs := &Configs{
					Project: []*Config{
						{},
					},
				}
				cfgs.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("project is required"))
			})

			t.Run("duplicate", func(t *ftt.Test) {
				cfgs := &Configs{
					Project: []*Config{
						{
							Project: "duplicated",
						},
						{
							Project: "unique",
						},
						{
							Project: "duplicated",
						},
					},
				}
				cfgs.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("is not unique"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				cfgs := &Configs{}
				cfgs.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})

			t.Run("project", func(t *ftt.Test) {
				cfgs := &Configs{
					Project: []*Config{
						{
							Project: "project",
						},
					},
				}
				cfgs.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})
		})
	})
}
