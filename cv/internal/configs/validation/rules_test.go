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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestValidationRules(t *testing.T) {
	t.Parallel()

	ftt.Run("Validation Rules", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "luci-change-verifier")
		r := validation.NewRuleSet()
		r.Vars.Register("appid", func(context.Context) (string, error) { return "luci-change-verifier", nil })

		addRules(r)

		patterns, err := r.ConfigPatterns(ctx)
		assert.NoErr(t, err)
		assert.Loosely(t, len(patterns), should.Equal(2))
		t.Run("project-scope cq.cfg", func(t *ftt.Test) {
			assert.Loosely(t, patterns[0].ConfigSet.Match("projects/xyz"), should.BeTrue)
			assert.Loosely(t, patterns[0].ConfigSet.Match("projects/xyz/refs/heads/master"), should.BeFalse)
			assert.Loosely(t, patterns[0].Path.Match("commit-queue.cfg"), should.BeTrue)
		})
		t.Run("service-scope listener-settings.cfg", func(t *ftt.Test) {
			assert.Loosely(t, patterns[1].ConfigSet.Match("services/luci-change-verifier"), should.BeTrue)
			assert.Loosely(t, patterns[1].ConfigSet.Match("projects/xyz/refs/heads/master"), should.BeFalse)
			assert.Loosely(t, patterns[1].Path.Match("listener-settings.cfg"), should.BeTrue)
		})
		t.Run("Dev", func(t *ftt.Test) {
			ctx = memory.UseWithAppID(context.Background(), "luci-change-verifier-dev")
			patterns, err := r.ConfigPatterns(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, patterns[0].Path.Match("commit-queue-dev.cfg"), should.BeTrue)
		})
	})
}

func mustHaveOnlySeverity(t testing.TB, err error, severity validation.Severity) error {
	assert.Loosely(t, err, should.NotBeNil)
	for _, e := range err.(*validation.Error).Errors {
		s, ok := validation.SeverityTag.In(e)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, s, should.Equal(severity))
	}
	return err
}

func mustWarn(t testing.TB, err error) error {
	return mustHaveOnlySeverity(t, err, validation.Warning)
}

func mustError(t testing.TB, err error) error {
	return mustHaveOnlySeverity(t, err, validation.Blocking)
}
