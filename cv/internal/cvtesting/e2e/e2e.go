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
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
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
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/impl"
	"go.chromium.org/luci/cv/internal/run"
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
//   ...
//   ct.RunUntil(ctx, func() bool { return len(ct.LoadRunsOf("project")) > 0 })
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
	tqSweepChannel dispatcher.Channel
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
	ctx, ctxCancel := t.Test.SetUp()
	deferme = ctxCancel

	if (*dsFlakinessFlag) != 0 {
		t.dsFlakiness = *dsFlakinessFlag
		if t.dsFlakiness < 0 || t.dsFlakiness > 1 {
			panic(fmt.Errorf("invalid %s %f: must be between 0.0 and 1.0", dsFlakinessFlagName, t.dsFlakiness))
		}
		logging.Warningf(ctx, "Using %.4f flaky Datastore", t.dsFlakiness)
		t.dsFlakinesRand = rand.NewSource(0)
		stopSweeping := t.startTQSweeping(ctx)
		deferme = func() {
			stopSweeping()
			ctxCancel()
		}
	}

	t.PMNotifier = prjmanager.NewNotifier(t.TQDispatcher)
	t.RunNotifier = run.NewNotifier(t.TQDispatcher)
	clUpdater := updater.New(t.TQDispatcher, t.PMNotifier, t.RunNotifier)
	_ = pmimpl.New(t.PMNotifier, t.RunNotifier, clUpdater)
	_ = runimpl.New(t.RunNotifier, t.PMNotifier, clUpdater)

	t.MigrationServer = &migration.MigrationServer{}
	t.DiagnosticServer = &diagnostic.DiagnosticServer{}
	return ctx, deferme
}

// RunUntil runs TQ tasks, while stopIf returns false.
//
// If `dsFlakinessFlag` is set, uses flaky datastore for running TQ tasks.
func (t *Test) RunUntil(ctx context.Context, stopIf func() bool) {
	maxTasks := 1000.0
	taskCtx := ctx
	if t.dsFlakiness > 0 {
		maxTasks *= math.Max(1.0, math.Round(1000*t.dsFlakiness))
		taskCtx = t.flakifyDS(ctx)
	}
	i := 0
	tooLong := false

	t.enqueueTQSweep(ctx)
	var finished []string
	t.TQ.Run(
		taskCtx,
		// StopAfter must be first and also always return false s.t. we correctly
		// record all finished tasks.
		tqtesting.StopAfter(func(task *tqtesting.Task) bool {
			finished = append(finished, fmt.Sprintf("%30s (attempt# %d)", task.Class, task.Attempts))
			t.enqueueTQSweep(ctx)
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

///////////////////////////////////////////////////////////////////////////////
// Methods to examine state.

// Now returns test clock time in UTC.
func (t *Test) Now() time.Time {
	return t.Clock.Now().UTC()
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

// MaxCQVote returns max CQ vote of a Gerrit CL loaded from Gerrit fake.
//
// Returns 0 if there are no votes.
// Panics if CL doesn't exist.
func (t *Test) MaxCQVote(ctx context.Context, gHost string, gChange int64) int32 {
	c := t.GFake.GetChange(gHost, int(gChange))
	if c == nil {
		panic(fmt.Errorf("%s/%d doesn't exist", gHost, gChange))
	}
	max := int32(0)
	for _, v := range c.Info.GetLabels()[trigger.CQLabelName].GetAll() {
		if v.GetValue() > max {
			max = v.GetValue()
		}
	}
	return max
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

///////////////////////////////////////////////////////////////////////////////
// DS flakiness & TQ sweep implementation.

// flakifyDS returns context with flaky Datastore.
func (t *Test) flakifyDS(ctx context.Context) context.Context {
	ctx, fb := featureBreaker.FilterRDS(ctx, nil)
	fb.BreakFeaturesWithCallback(
		flaky.Errors(flaky.Params{
			Rand:                t.dsFlakinesRand,
			DeadlineProbability: t.dsFlakiness,
		}),
		"DecodeCursor",
		"Run",
		"Count",
		"BeginTransaction",
		"GetMulti",
	)
	// NOTE: feature breaker currently doesn't allow simulating
	// an error from actually successful DeleteMulti/PutMulti, a.k.a. submarine
	// writes. However, the CommitTransaction feature breaker is simulating
	// returning an error from an actually successful transaction, which makes
	// ConcurrentTransactionProbability incorrectly simulated.
	//
	// NOTE: a transaction with 1 Get and 1 Put will roll a dice 4 times:
	//   BeginTransaction, GetMulti, PutMulti, CommitTransaction.
	//   However, in Cloud Datastore client, PutMulti within transaction doesn't
	//   reach Datastore until the end of the transaction.
	//
	// TODO(tandrii): make realistic feature breaker with submarine writes outside
	// of transaction and easier to control probabilities of transaction breaker.
	//
	// For now, use 5x higher probability of failure for mutations,
	// which for simple Get/Put transactions results (2x + 10x) higher probability
	// than a non-transactional Get.
	fb.BreakFeaturesWithCallback(
		flaky.Errors(flaky.Params{
			Rand:                             t.dsFlakinesRand,
			DeadlineProbability:              math.Min(t.dsFlakiness*5, 1.0),
			ConcurrentTransactionProbability: 0,
		}),
		"AllocateIDs",
		"DeleteMulti",
		"PutMulti",
		"CommitTransaction",
	)
	return ctx
}

// startTQSweeping starts asynchronous sweeping for the duration of the test.
//
// This is necessary if flaky DS is used.
func (t *Test) startTQSweeping(ctx context.Context) (deferme func()) {
	t.TQDispatcher.Sweeper = tq.NewInProcSweeper(tq.InProcSweeperOptions{
		SweepShards:     1,
		SubmitBatchSize: 1,
	})
	var err error
	t.tqSweepChannel, err = dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			Buffer: buffer.Options{
				BatchItemsMax: 1, // incoming event => sweep ASAP.
				MaxLeases:     1, // at most 1 sweep concurrently
				// 2+ outstanding requests to sweep should result in just 1 sweep.
				FullBehavior: &buffer.DropOldestBatch{MaxLiveItems: 1},
				// This is only useful if something is misconfigured to avoid pointless
				// retries, because the individual sweeps must not fail as we use
				// non-flaky Datastore for sweeping.
				Retry: retry.None,
			},
		},
		func(*buffer.Batch) error { return t.TQDispatcher.Sweep(ctx) },
	)
	if err != nil {
		panic(err)
	}
	return func() { t.tqSweepChannel.CloseAndDrain(ctx) }
}

// enqueueTQSweep ensures a TQ sweep will happen strictly afterwards.
//
// Noop if TQ sweeping is not required.
func (t *Test) enqueueTQSweep(ctx context.Context) {
	if t.TQDispatcher.Sweeper != nil {
		t.tqSweepChannel.C <- struct{}{}
	}
}
