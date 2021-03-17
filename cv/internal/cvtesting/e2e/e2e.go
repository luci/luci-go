// Copyright 2021 The LUCI Authors.
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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/diagnostic"
	pollertask "go.chromium.org/luci/cv/internal/gerrit/poller/task"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	// Import all modules with server/tq handler additions in init() calls, which
	// are otherwise not imported directly or transitively via imports above.
	_ "go.chromium.org/luci/cv/internal/prjmanager/impl"
	_ "go.chromium.org/luci/cv/internal/run/impl"
)

// Test encapsulates e2e setup for a CV test.
//
// Embeds cvtesting.Test, which sets CV's dependencies and some simple CV
// components (e.g. TreeClient), while this Test focuses on setup of CV's own
// components.
//
// Typical use:
//   ct := Test{CVDev: true}
//   ctx, cancel := ct.SetUp()
//   defer cancel()
type Test struct {
	*cvtesting.Test // auto-initialized if nil
	// CVDev if true sets e2e test to use `cv-dev` GAE app.
	// Defaults to `cv` GAE app.
	CVDev bool

	DiagnosticServer diagnosticpb.DiagnosticServer
	MigrationServer  migrationpb.MigrationServer
	// TODO(tandrii): add CQD fake.
}

func (t *Test) SetUp() (ctx context.Context, deferme func()) {
	switch {
	case t.Test == nil:
		t.Test = &cvtesting.Test{}
	case t.Test.AppID != "":
		panic("overriding cvtesting.Test{AppID} in e2e not supported")
	}
	switch t.CVDev {
	case true:
		t.Test.AppID = "cv-dev"
	case false:
		t.Test.AppID = "cv"
	}

	// Delegate most setup to cvtesting.Test.
	ctx, cancel := t.Test.SetUp()

	t.MigrationServer = &migration.MigrationServer{}
	t.DiagnosticServer = &diagnostic.DiagnosticServer{}
	return ctx, cancel
}

// Now returns test clock time in UTC.
func (t *Test) Now() time.Time {
	return t.Clock.Now().UTC()
}

// RunAtLeastOncePM runs at least 1 PM task, possibly more or other tasks.
func (t *Test) RunAtLeastOncePM(ctx context.Context) {
	t.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
}

// RunAtLeastOnceRun runs at least 1 Run task, possibly more or other tasks.
func (t *Test) RunAtLeastOnceRun(ctx context.Context) {
	t.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
}

// RunAtLeastOncePoller runs at least 1 Poller task, possibly more or other
// tasks.
func (t *Test) RunAtLeastOncePoller(ctx context.Context) {
	t.TQ.Run(ctx, tqtesting.StopAfterTask(pollertask.ClassID))
}

// RunUntil runs TQ tasks, while stopIf returns false.
func (t *Test) RunUntil(ctx context.Context, stopIf func() bool) {
	// TODO(tandrii): adjust based on concurrent/serial execution and flakiness of
	// Datastore.
	const maxTasks = 1000
	i := 0
	// Use StopBefore such that first check is before running any task.
	t.TQ.Run(ctx, tqtesting.StopBefore(func(t *tqtesting.Task) bool {
		switch {
		case stopIf():
			return true
		case i == maxTasks:
			return true
		default:
			i++
			return false
		}
	}))
	// Log only here after all tasks-in-progress are completed.
	logging.Debugf(ctx, "RunUntil ran %d iterations", i)
	if i == maxTasks {
		panic("RunUntil ran for too long!")
	}
}

// LoadProject returns Project entity or nil if not exists.
func (t *Test) LoadProject(ctx context.Context, lProject string) *prjmanager.Project {
	p, err := prjmanager.Load(ctx, lProject)
	if err != nil {
		panic(err)
	}
	return p
}

// LoadRun returns Run entity or nil if not exists.
func (t *Test) LoadRun(ctx context.Context, id common.RunID) *run.Run {
	r := &run.Run{ID: id}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil
	case err != nil:
		panic(err)
	default:
		return r
	}
}

// LoadRunsOf loads all Runs of a project from Datastore.
func (t *Test) LoadRunsOf(ctx context.Context, lProject string) []*run.Run {
	var res []*run.Run
	err := datastore.GetAll(ctx, run.NewQueryWithLUCIProject(ctx, lProject), &res)
	if err != nil {
		panic(err)
	}
	return res
}

// EarliestCreatedRun returns the earliest created Run in a project.
//
// If there are several such runs, may return any one of them.
//
// Returns nil if there are no Runs.
func (t *Test) EarliestCreatedRunOf(ctx context.Context, lProject string) *run.Run {
	var earliest *run.Run
	for _, r := range t.LoadRunsOf(ctx, lProject) {
		if earliest == nil || earliest.CreateTime.After(r.CreateTime) {
			earliest = r
		}
	}
	return earliest
}

// LoadCL returns CL entity or nil if not exists.
func (t *Test) LoadCL(ctx context.Context, id common.CLID) *changelist.CL {
	cl := &changelist.CL{ID: id}
	switch err := datastore.Get(ctx, cl); {
	case err == datastore.ErrNoSuchEntity:
		return nil
	case err != nil:
		panic(err)
	default:
		return cl
	}
}

// LoadGerritCL returns CL entity or nil if not exists.
func (t *Test) LoadGerritCL(ctx context.Context, gHost string, gChange int64) *changelist.CL {
	switch cl, err := changelist.MustGobID(gHost, gChange).Get(ctx); {
	case err == datastore.ErrNoSuchEntity:
		return nil
	case err != nil:
		panic(err)
	default:
		return cl
	}
}

// LogPhase emits easy to recognize log like
// ===========================
// PHASE: ....
// ===========================
func (t *Test) LogPhase(ctx context.Context, format string, args ...interface{}) {
	line := strings.Repeat("=", 80)
	format = fmt.Sprintf("\n%s\nPHASE: %s\n%s", line, format, line)
	logging.Debugf(ctx, format, args...)
}

// MakeCfgSingular return project config with a single ConfigGroup.
func MakeCfgSingular(cgName, gHost, gRepo, gRef string) *cfgpb.Config {
	return &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: cgName,
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://" + gHost + "/",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      gRepo,
								RefRegexp: []string{gRef},
							},
						},
					},
				},
			},
		},
	}
}
