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

	. "github.com/smartystreets/goconvey/convey"

	configpb "go.chromium.org/luci/bisection/proto/config"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCanCreateRevert(t *testing.T) {
	ctx := memory.Use(context.Background())

	datastore.GetTestable(ctx).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_created",
				},
				{
					Property: "revert_create_time",
				},
			},
		},
	)
	datastore.GetTestable(ctx).CatchupIndexes()

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	ctx = clock.Set(ctx, cl)

	Convey("Disabling all Gerrit actions should override CanCreateRevert to false", t, func() {
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
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg)
		So(err, ShouldBeNil)
		So(canCreate, ShouldEqual, false)
		So(reason, ShouldEqual, "all Gerrit actions are disabled")
	})

	Convey("CanCreateRevert should be false when create is disabled", t, func() {
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
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg)
		So(err, ShouldBeNil)
		So(canCreate, ShouldEqual, false)
		So(reason, ShouldEqual, "LUCI Bisection's revert creation has been disabled")
	})

	Convey("CanCreateRevert should be true", t, func() {
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
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg)
		So(err, ShouldBeNil)
		So(canCreate, ShouldEqual, true)
		So(reason, ShouldEqual, "")
	})

	Convey("CanCreateRevert should be false when daily limit has been reached", t, func() {
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
		canCreate, reason, err := CanCreateRevert(ctx, gerritCfg)
		So(err, ShouldBeNil)
		So(canCreate, ShouldEqual, false)
		So(reason, ShouldEqual, "LUCI Bisection's daily limit for revert creation"+
			" (0) has been reached; 0 reverts have already been created")
	})
}

func TestCanSubmitRevert(t *testing.T) {
	ctx := memory.Use(context.Background())

	datastore.GetTestable(ctx).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_committed",
				},
				{
					Property: "revert_commit_time",
				},
			},
		},
	)
	datastore.GetTestable(ctx).CatchupIndexes()

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	ctx = clock.Set(ctx, cl)

	Convey("Disabling all Gerrit actions should override CanSubmitRevert to false", t, func() {
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
		So(err, ShouldBeNil)
		So(canSubmit, ShouldEqual, false)
		So(reason, ShouldEqual, "all Gerrit actions are disabled")
	})

	Convey("CanSubmitRevert should be false when submit is disabled", t, func() {
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
		So(err, ShouldBeNil)
		So(canSubmit, ShouldEqual, false)
		So(reason, ShouldEqual, "LUCI Bisection's revert submission has been disabled")
	})

	Convey("CanSubmitRevert should be true", t, func() {
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
		So(err, ShouldBeNil)
		So(canSubmit, ShouldEqual, true)
		So(reason, ShouldEqual, "")
	})

	Convey("CanSubmitRevert should be false when daily limit has been reached", t, func() {
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
		So(err, ShouldBeNil)
		So(canSubmit, ShouldEqual, false)
		So(reason, ShouldEqual, "LUCI Bisection's daily limit for revert submission"+
			" (0) has been reached; 0 reverts have already been submitted")
	})
}
