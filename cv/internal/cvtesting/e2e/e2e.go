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

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/admin"
	adminpb "go.chromium.org/luci/cv/internal/admin/api"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/migration/cqdfake"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/manager"
	"go.chromium.org/luci/cv/internal/run"
	runbq "go.chromium.org/luci/cv/internal/run/bq"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
)

const dsFlakinessFlagName = "cv.dsflakiness"
const tqConcurrentFlagName = "cv.tqparallel"
const extraVerboseFlagName = "cv.verbose"

var dsFlakinessFlag = flag.Float64(dsFlakinessFlagName, 0, "DS flakiness probability between 0(default) and 1.0 (always fails)")
var tqParallelFlag = flag.Bool(tqConcurrentFlagName, false, "Runs TQ tasks in parallel")
var extraVerbosityFlag = flag.Bool(extraVerboseFlagName, false, "Extra verbose mode. Use in combination with -v")

func init() {
	// HACK: Bump up greatly eventbox tombstone delay, especially useful in case
	// of tqParallelFlag: the fake test clock is ran at much much
	// higher speed than real clock, which results in spurious stale eventbox
	// listing errors.
	eventbox.TombstonesDelay = time.Hour
}

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

	AdminServer     adminpb.AdminServer
	MigrationServer migrationpb.MigrationServer

	// dsFlakiness enables ds flakiness for "RunUntil".
	dsFlakiness    float64
	dsFlakinesRand rand.Source
	tqSweepChannel dispatcher.Channel

	cqdsMu sync.Mutex
	// cqds are fake CQDaemons indexed by LUCI project name.
	cqds map[string]*cqdfake.CQDFake
}

// SetUp sets up the end to end test.
//
// Must be called exactly once.
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

	gFactory := t.GFactory()
	t.PMNotifier = prjmanager.NewNotifier(t.TQDispatcher)
	t.RunNotifier = run.NewNotifier(t.TQDispatcher)
	clMutator := changelist.NewMutator(t.TQDispatcher, t.PMNotifier, t.RunNotifier)
	clUpdater := updater.New(t.TQDispatcher, gFactory, &gerrit.MirrorIteratorFactory{}, clMutator)
	_ = pmimpl.New(t.PMNotifier, t.RunNotifier, clMutator, gFactory, clUpdater)
	_ = runimpl.New(t.RunNotifier, t.PMNotifier, clMutator, clUpdater, gFactory, t.TreeFake.Client(), t.BQFake)

	t.MigrationServer = &migration.MigrationServer{
		RunNotifier: t.RunNotifier,
		GFactory:    gFactory,
	}
	t.AdminServer = &admin.AdminServer{
		TQDispatcher:  t.TQDispatcher,
		RunNotifier:   t.RunNotifier,
		PMNotifier:    t.PMNotifier,
		GerritUpdater: clUpdater,
	}
	return ctx, deferme
}

// RunUntil runs TQ tasks, while stopIf returns false.
//
// If `dsFlakinessFlag` is set, uses flaky datastore for running TQ tasks.
// If `tqParallelFlag` is set, runs TQ tasks concurrently.
//
// Not goroutine safe.
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
	tqOpts := []tqtesting.RunOption{
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
	}
	if *tqParallelFlag {
		tqOpts = append(tqOpts, tqtesting.ParallelExecute())
	}
	t.TQ.Run(taskCtx, tqOpts...)

	// Log only here after all tasks-in-progress are completed.
	outstanding := stringset.New(len(t.TQ.Tasks().Pending()))
	for _, task := range t.TQ.Tasks().Pending() {
		outstanding.Add(task.Class)
	}
	logging.Debugf(ctx, "RunUntil ran %d iterations, finished %d tasks, left %d tasks: %s", i, len(finished), len(outstanding), outstanding.ToSortedSlice())
	if *extraVerbosityFlag {
		for i, v := range finished {
			logging.Debugf(ctx, "    finished #%d task: %s", i, v)
		}
	}
	if tooLong {
		panic(errors.New("RunUntil ran for too long!"))
	}
	if err := ctx.Err(); err != nil {
		panic(err)
	}
}

// MustCQD returns a CQDaemon fake for the given project, starting a new one if
// necessary, in which case it's lifetime is limited by the given context.
func (t *Test) MustCQD(ctx context.Context, luciProject string) *cqdfake.CQDFake {
	t.cqdsMu.Lock()
	defer t.cqdsMu.Unlock()
	if cqd, exists := t.cqds[luciProject]; exists {
		return cqd
	}
	cqd := &cqdfake.CQDFake{
		LUCIProject: luciProject,
		CV:          t.MigrationServer,
		GFake:       t.GFake,
	}
	cqd.Start(ctx)
	if t.cqds == nil {
		t.cqds = make(map[string]*cqdfake.CQDFake, 1)
	}
	t.cqds[luciProject] = cqd
	return cqd
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

// LoadGerritRuns loads all Runs from Datastore which include a Gerrit CL.
func (t *Test) LoadGerritRuns(ctx context.Context, gHost string, gChange int64, lProject string) []*run.Run {
	// TODO(tandrii): use query based on CL ID and don't require lProject
	// argument.
	cl := t.LoadGerritCL(ctx, gHost, gChange)
	if cl == nil {
		return nil
	}
	var runs []*run.Run
	err := datastore.GetAll(ctx, run.NewQueryWithLUCIProject(ctx, lProject), &runs)
	if err != nil {
		panic(err)
	}
	res := runs[:0]
	for _, r := range runs {
		for _, clid := range r.CLs {
			if clid == cl.ID {
				res = append(res, r)
				break
			}
		}
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

// LatestRunWithCL returns the latest created Run containing given CL.
//
// If there are several, returns the one with latest .StartTime.
// Returns nil if there is such Runs, including if Gerrit CL isn't yet in DS.
func (t *Test) LatestRunWithGerritCL(ctx context.Context, lProject, gHost string, gChange int64) *run.Run {
	cl := t.LoadGerritCL(ctx, gHost, gChange)
	if cl == nil {
		return nil
	}
	var ret *run.Run
	for _, r := range t.LoadRunsOf(ctx, lProject) {
		for _, clid := range r.CLs {
			switch {
			case clid != cl.ID:
			case ret == nil:
				ret = r
			case ret.CreateTime.After(r.CreateTime):
				ret = r
			case ret.CreateTime.Equal(r.CreateTime) && ret.StartTime.Before(r.StartTime):
				ret = r
			}
		}
	}
	return ret
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

// MaxVote returns max vote of a Gerrit CL loaded from Gerrit fake.
//
// Returns 0 if there are no votes.
// Panics if CL doesn't exist.
func (t *Test) MaxVote(ctx context.Context, gHost string, gChange int64, gLabel string) int32 {
	c := t.GFake.GetChange(gHost, int(gChange))
	if c == nil {
		panic(fmt.Errorf("%s/%d doesn't exist", gHost, gChange))
	}
	max := int32(0)
	for _, v := range c.Info.GetLabels()[gLabel].GetAll() {
		if v.GetValue() > max {
			max = v.GetValue()
		}
	}
	return max
}

// MaxCQVote returns max CQ vote of a Gerrit CL loaded from Gerrit fake.
//
// Returns 0 if there are no votes.
// Panics if CL doesn't exist.
func (t *Test) MaxCQVote(ctx context.Context, gHost string, gChange int64) int32 {
	return t.MaxVote(ctx, gHost, gChange, trigger.CQLabelName)
}

// LastMessage returns the last message posted on a Gerrit CL from Gerrit fake.
//
// Returns nil if there are no messages.
// Panics if the CL doesn't exist.
func (t *Test) LastMessage(gHost string, gChange int64) *gerritpb.ChangeMessageInfo {
	return gf.LastMessage(t.GFake.GetChange(gHost, int(gChange)).Info)
}

// ExportedBQAttemptsCount returns number of exported CQ Attempts.
func (t *Test) ExportedBQAttemptsCount() int {
	return t.BQFake.RowsCount("", runbq.CVDataset, runbq.CVTable)
}

func (t *Test) MigrationFetchActiveRuns(ctx context.Context, project string) []*migrationpb.ActiveRun {
	req := &migrationpb.FetchActiveRunsRequest{LuciProject: project}
	res, err := t.MigrationServer.FetchActiveRuns(t.MigrationContext(ctx), req)
	if err != nil {
		panic(err)
	}
	return res.GetActiveRuns()
}

// MigrationContext returns context authorized to call to the Migration API.
func (t *Test) MigrationContext(ctx context.Context) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:e2e-test@example.com",
		IdentityGroups: []string{migration.AllowGroup},
	})
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

// MakeCfgCombinable return project config with a combinable ConfigGroup.
func MakeCfgCombinable(cgName, gHost, gRepo, gRef string) *cfgpb.Config {
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
				CombineCls: &cfgpb.CombineCLs{
					StabilizationDelay: durationpb.New(5 * time.Minute),
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
	// NOTE: Feature breaker currently doesn't allow simulating
	// an error from actually successful DeleteMulti/PutMulti, a.k.a. submarine
	// writes. However, the CommitTransaction feature breaker is simulating
	// returning an error from an actually successful transaction, which makes
	// ConcurrentTransactionProbability incorrectly simulated.
	//
	// NOTE: A transaction with 1 Get and 1 Put will roll a dice 4 times:
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
