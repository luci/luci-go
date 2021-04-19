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
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/diagnostic"
	pollertask "go.chromium.org/luci/cv/internal/gerrit/poller/task"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/impl"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
)

const dsFlakinessFlagName = "cv.dsflakiness"

var dsFlakinessFlag = flag.Float64(dsFlakinessFlagName, 0, "DS flakiness probability between 0(default) and 1.0 (always fails)")

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

	PMNotifier  *prjmanager.Notifier
	RunNotifier *run.Notifier

	DiagnosticServer diagnosticpb.DiagnosticServer
	MigrationServer  migrationpb.MigrationServer
	// TODO(tandrii): add CQD fake.

	// dsFlakiness enables ds flakiness for "RunUntil".
	dsFlakiness    float64
	dsFlakinesRand rand.Source
}

func (t *Test) SetUp() (ctx context.Context, deferme func()) {
	switch {
	case t.Test == nil:
		t.Test = &cvtesting.Test{
			TQDispatcher: &tq.Dispatcher{},
		}
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

	if (*dsFlakinessFlag) != 0 {
		t.dsFlakiness = *dsFlakinessFlag
		if t.dsFlakiness < 0 || t.dsFlakiness > 1 {
			panic(fmt.Errorf("invalid %s %f: must be between 0.0 and 1.0", dsFlakinessFlagName, t.dsFlakiness))
		}
		logging.Warningf(ctx, "Using %.4f flaky Datastore", t.dsFlakiness)
		t.dsFlakinesRand = rand.NewSource(0)
	}

	t.PMNotifier = prjmanager.NewNotifier(t.TQDispatcher)
	t.RunNotifier = run.NewNotifier(t.TQDispatcher)
	clUpdater := updater.New(t.TQDispatcher, t.PMNotifier, t.RunNotifier)
	_ = pmimpl.New(t.PMNotifier, t.RunNotifier, clUpdater)
	_ = runimpl.New(t.RunNotifier, t.PMNotifier, clUpdater)

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
//
// If `dsFlakinessFlag` is set, uses flaky datastore for running TQ tasks.
func (t *Test) RunUntil(ctx context.Context, stopIf func() bool) {
	maxTasks := 1000.0
	taskCtx := ctx
	if t.dsFlakiness > 0 {
		maxTasks *= math.Max(1.0, math.Round(100*t.dsFlakiness))
		taskCtx = t.flakifyDS(ctx)
	}
	i := 0
	tooLong := false

	t.sweepTTQ(ctx)
	var finished []string
	t.TQ.Run(
		taskCtx,
		// StopAfter must be first and also always return false s.t. we correctly
		// record all finished tasks.
		tqtesting.StopAfter(func(task *tqtesting.Task) bool {
			finished = append(finished, fmt.Sprintf("%30s (attempt# %d)", task.Class, task.Attempts))
			t.sweepTTQ(ctx)
			return false
		}),
		// StopBefore is actually used to for conditional stopping.
		// Note that it can `return true` (meaning stop) before any task was run at
		// all.
		tqtesting.StopBefore(func(t *tqtesting.Task) bool {
			switch {
			case stopIf():
				return true
			case float64(i) >= maxTasks:
				tooLong = true
				return true
			default:
				i++
				if i%1000 == 0 {
					logging.Debugf(ctx, "RunUntil is running %d task", i)
				}
				return false
			}
		}),
	)

	// Log only here after all tasks-in-progress are completed.
	outstanding := make([]string, len(t.TQ.Tasks().Pending()))
	for i, task := range t.TQ.Tasks().Pending() {
		outstanding[i] = task.Class
	}
	logging.Debugf(ctx, "RunUntil ran %d iterations, finished %d tasks, left %d tasks", i, len(finished), len(outstanding))
	for i, v := range finished {
		logging.Debugf(ctx, "    finished #%d task: %s", i, v)
	}
	if len(outstanding) > 0 {
		logging.Debugf(ctx, "  outstanding: %s", outstanding)
	}

	if tooLong {
		panic(errors.New("RunUntil ran for too long!"))
	}
	if err := ctx.Err(); err != nil {
		panic(err)
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

// implementation detail

// flakifyDS returns context with flaky Datastore.
func (t *Test) flakifyDS(ctx context.Context) context.Context {
	ctx, fb := featureBreaker.FilterRDS(ctx, nil)
	fb.BreakFeaturesWithCallback(
		flaky.Errors(flaky.Params{
			Rand:                             t.dsFlakinesRand,
			DeadlineProbability:              t.dsFlakiness,
			ConcurrentTransactionProbability: t.dsFlakiness,
		}),
		featureBreaker.DatastoreFeatures...,
	)
	return ctx
}

// sweepTTQ ensures all previously transactionally created tasks are actually
// added to TQ.
//
// This is critical when datastore is flaky, as this may result in transaction
// succeeding, but client receiving a transient error.
//
// Context passed must not have flaky datastore.
func (t *Test) sweepTTQ(ctx context.Context) {
	if t.dsFlakiness > 0 {
		// TODO(tandrii): find a way to instantiate && launch Sweeper per test with
		// a controlled lifetime.
		// Currently, tq.Default.Sweeper is global AND allows at most 1 concurrent
		// sweep, regardless of what `ctx` is passed to Sweep.
		// Furthermore, Sweep must be ran outside of
		// tqtesting.StopAfter/tqtesting.StopBefore callbacks to avoid deadlock.
		go func() {
			tqDefaultSweeperLock.Lock()
			defer tqDefaultSweeperLock.Unlock()
			if err := tq.Sweep(ctx); err != nil && err != ctx.Err() {
				logging.Errorf(ctx, "sweep failed: %s", err)
			}
		}()
	}
}

var tqDefaultSweeperLock sync.Mutex

func init() {
	tq.Default.Sweeper = tq.NewInProcSweeper(tq.InProcSweeperOptions{
		SweepShards:     1,
		SubmitBatchSize: 1,
	})
}
