// Copyright 2022 The LUCI Authors.
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

	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestCanCreateRevert(t *testing.T) {
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	ctx = clock.Set(ctx, cl)

	ftt.Run("Disabling all Gerrit actions should override CanCreateRevert to false", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: false,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canCreate, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("all Gerrit actions are disabled"))
	})

	ftt.Run("CanCreateRevert should be false when create is disabled", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    false,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canCreate, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("LUCI Bisection's revert creation has been disabled"))
	})

	ftt.Run("CanCreateRevert should be true", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canCreate, should.Equal(true))
		assert.Loosely(t, reason, should.BeEmpty)
	})

	ftt.Run("CanCreateRevert should be false when daily limit has been reached", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 0,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canCreate, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("LUCI Bisection's daily limit for revert creation"+
			" (0) has been reached; 0 reverts have already been created"))
	})
}

func TestCanSubmitRevert(t *testing.T) {
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	ctx = clock.Set(ctx, cl)

	ftt.Run("Disabling all Gerrit actions should override CanSubmitRevert to false", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: false,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canSubmit, reason, err := CanSubmitRevert(ctx, gerritCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canSubmit, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("all Gerrit actions are disabled"))
	})

	ftt.Run("CanSubmitRevert should be false when submit is disabled", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    false,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canSubmit, reason, err := CanSubmitRevert(ctx, gerritCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canSubmit, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("LUCI Bisection's revert submission has been disabled"))
	})

	ftt.Run("CanSubmitRevert should be true", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canSubmit, reason, err := CanSubmitRevert(ctx, gerritCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canSubmit, should.Equal(true))
		assert.Loosely(t, reason, should.BeEmpty)
	})

	ftt.Run("CanSubmitRevert should be false when daily limit has been reached", t, func(t *ftt.Test) {
		gerritCfg := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 0,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
		}
		canSubmit, reason, err := CanSubmitRevert(ctx, gerritCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, canSubmit, should.Equal(false))
		assert.Loosely(t, reason, should.Equal("LUCI Bisection's daily limit for revert submission"+
			" (0) has been reached; 0 reverts have already been submitted"))
	})
}
