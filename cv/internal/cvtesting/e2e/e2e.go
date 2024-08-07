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
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	bblistener "go.chromium.org/luci/cv/internal/buildbucket/listener"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	gerritupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/manager"
	"go.chromium.org/luci/cv/internal/quota"
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	runbq "go.chromium.org/luci/cv/internal/run/bq"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
	cvpubsub "go.chromium.org/luci/cv/internal/run/pubsub"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/run/runquery"
	"go.chromium.org/luci/cv/internal/tryjob"
	"go.chromium.org/luci/cv/internal/tryjob/tjcancel"
	tjupdate "go.chromium.org/luci/cv/internal/tryjob/update"
)

const (
	dsFlakinessFlagName  = "cv.dsflakiness"
	tqConcurrentFlagName = "cv.tqparallel"
	extraVerboseFlagName = "cv.verbose"
	fastClockFlagName    = "cv.fastclock"
	// TODO(crbug/1344711): Change to a dev host or an example host.
	buildbucketHost    = chromeinfra.BuildbucketHost
	committers         = "committer-group"
	dryRunners         = "dry-runner-group"
	newPatchsetRunners = "new-patchset-runner-group"
)

var (
	dsFlakinessFlag    = flag.Float64(dsFlakinessFlagName, 0, "DS flakiness probability between 0(default) and 1.0 (always fails)")
	tqParallelFlag     = flag.Bool(tqConcurrentFlagName, false, "Runs TQ tasks in parallel")
	extraVerbosityFlag = flag.Bool(extraVerboseFlagName, false, "Extra verbose mode. Use in combination with -v")
	fastClockFlag      = flag.Int(fastClockFlagName, 0, "Use FastClock running at this multiplier over physical clock")
)

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
//
//	ct := Test{CVDev: true}
//	ctx, cancel := ct.SetUp(t)
//	defer cancel()
//	...
//	ct.RunUntil(ctx, func() bool { return len(ct.LoadRunsOf("project")) > 0 })
type Test struct {
	*cvtesting.Test // auto-initialized if nil

	PMNotifier  *prjmanager.Notifier
	RunNotifier *run.Notifier

	AdminServer adminpb.AdminServer

	// dsFlakiness enables ds flakiness for "RunUntil".
	dsFlakiness     float64
	dsFlakinessRand rand.Source
	tqSweepChannel  dispatcher.Channel[struct{}]
}

// SetUp sets up the end to end test.
//
// Must be called exactly once.
func (t *Test) SetUp(testingT *testing.T) context.Context {
	if t.Test == nil {
		t.Test = &cvtesting.Test{}
	}

	switch speedUp := *fastClockFlag; {
	case speedUp < 0:
		panic(fmt.Errorf("invalid %s %d: must be >= 0", fastClockFlagName, speedUp))
	case speedUp > 0:
		t.Clock = testclock.NewFastClock(
			time.Date(2020, time.February, 2, 13, 30, 00, 0, time.FixedZone("Fake local", 3*60*60)),
			speedUp)
	default:
		// Use default testclock.
	}

	// Delegate most setup to cvtesting.Test.
	ctx := t.Test.SetUp(testingT)
	t.Test.DisableProjectInGerritListener(ctx, ".*")

	if (*dsFlakinessFlag) != 0 {
		t.dsFlakiness = *dsFlakinessFlag
		if t.dsFlakiness < 0 || t.dsFlakiness > 1 {
			panic(fmt.Errorf("invalid %s %f: must be between 0.0 and 1.0", dsFlakinessFlagName, t.dsFlakiness))
		}
		logging.Warningf(ctx, "Using %.4f flaky Datastore", t.dsFlakiness)
		t.dsFlakinessRand = rand.NewSource(0)
		testingT.Cleanup(func() { t.startTQSweeping(ctx) })
	}

	gFactory := t.GFactory()
	t.PMNotifier = prjmanager.NewNotifier(t.TQDispatcher)
	t.RunNotifier = run.NewNotifier(t.TQDispatcher)
	tjNotifier := tryjob.NewNotifier(t.TQDispatcher)
	clMutator := changelist.NewMutator(t.TQDispatcher, t.PMNotifier, t.RunNotifier, tjNotifier)
	clUpdater := changelist.NewUpdater(t.TQDispatcher, clMutator)
	bbFactory := t.BuildbucketFake.NewClientFactory()
	topic, sub, cleanupFn := t.makeBuildbucketPubsub(ctx)
	testingT.Cleanup(cleanupFn)
	t.BuildbucketFake.RegisterPubsubTopic(buildbucketHost, topic)
	cleanupFn = bblistener.StartListenerForTest(ctx, sub, tjNotifier)
	testingT.Cleanup(cleanupFn)
	qm := quota.NewManager(gFactory)
	gerritupdater.RegisterUpdater(clUpdater, gFactory)
	rdbFactory := rdb.NewMockRecorderClientFactory(t.Test.GoMockCtl)
	_ = pmimpl.New(t.PMNotifier, t.RunNotifier, clMutator, gFactory, clUpdater)
	_ = runimpl.New(t.RunNotifier, t.PMNotifier, tjNotifier, clMutator, clUpdater, gFactory, bbFactory, t.TreeFake.Client(), t.BQFake, rdbFactory, qm, t.Env)
	bbFacade := &bbfacade.Facade{
		ClientFactory: bbFactory,
	}
	tryjobUpdater := tjupdate.NewUpdater(t.Env, tjNotifier, t.RunNotifier)
	tryjobUpdater.RegisterBackend(bbFacade)
	tryjobCancellator := tjcancel.NewCancellator(tjNotifier)
	tryjobCancellator.RegisterBackend(bbFacade)
	t.AdminServer = admin.New(t.TQDispatcher, &dsmapper.Controller{}, clUpdater, t.PMNotifier, t.RunNotifier)
	return ctx
}

// RunUntil runs TQ tasks, while stopIf returns false.
//
// If `dsFlakinessFlag` is set, uses flaky datastore for running TQ tasks.
// If `tqParallelFlag` is set, runs TQ tasks concurrently.
//
// Not goroutine safe.
func (t *Test) RunUntil(ctx context.Context, stopIf func() bool) {
	t.RunUntilT(ctx, 100, stopIf)
}

// RunUntilT is the same as RunUntil but with custom approximate number of tasks
// if ran in serial mode.
//
// Depending on command line test options, allows different number of tasks
// executions.
func (t *Test) RunUntilT(ctx context.Context, targetTasksCount int, stopIf func() bool) {
	// Default to 10x targetTasksCount tasks s.t. test writers don't need to
	// calculate exactly how many tasks they need and also to be more robust
	// against future changes in CV impl which may slightly increase number of TQ
	// tasks.
	maxTasks := 10 * float64(targetTasksCount)
	taskCtx := ctx
	if t.dsFlakiness > 0 {
		maxTasks *= math.Max(1.0, math.Round(1000*t.dsFlakiness))
		taskCtx = t.flakifyDS(ctx)
	}
	if *tqParallelFlag {
		// Executing tasks concurrently usually leads to Datastore and other
		// contention, so allow more retries.
		maxTasks *= 10
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
	r, err := run.LoadRun(ctx, id)
	if err != nil {
		panic(err)
	}
	return r
}

// LoadRunsOf loads all Runs of a project from Datastore.
func (t *Test) LoadRunsOf(ctx context.Context, lProject string) []*run.Run {
	runs, _, err := runquery.ProjectQueryBuilder{Project: lProject}.LoadRuns(ctx)
	if err != nil {
		panic(err)
	}
	return runs
}

// LoadGerritRuns loads all Runs from Datastore which include a Gerrit CL.
func (t *Test) LoadGerritRuns(ctx context.Context, gHost string, gChange int64) []*run.Run {
	cl := t.LoadGerritCL(ctx, gHost, gChange)
	if cl == nil {
		return nil
	}
	runs, _, err := runquery.CLQueryBuilder{CLID: cl.ID}.LoadRuns(ctx)
	if err != nil {
		panic(err)
	}
	return runs
}

// EarliestCreatedRunOf returns the earliest created Run in a project.
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

// LatestRunWithGerritCL returns the latest created Run containing given CL.
//
// If there are several, returns the one with latest .StartTime.
// Returns nil if there is such Runs, including if Gerrit CL isn't yet in DS.
func (t *Test) LatestRunWithGerritCL(ctx context.Context, gHost string, gChange int64) *run.Run {
	var ret *run.Run
	for _, r := range t.LoadGerritRuns(ctx, gHost, gChange) {
		switch {
		case ret == nil:
			ret = r
		case ret.CreateTime.After(r.CreateTime):
			ret = r
		case ret.CreateTime.Equal(r.CreateTime) && ret.StartTime.Before(r.StartTime):
			ret = r
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
	cl, err := changelist.MustGobID(gHost, gChange).Load(ctx)
	if err != nil {
		panic(err)
	}
	return cl
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

// AddCommitter adds a given member into the committer group.
func (t *Test) AddCommitter(email string) {
	t.AddMember(email, committers)
}

// AddDryRunner adds a given member into the dry-runner group.
func (t *Test) AddDryRunner(email string) {
	t.AddMember(email, dryRunners)
}

// AddNewPatchsetRunner adds a given member into the new-patchset-runner group.
func (t *Test) AddNewPatchsetRunner(email string) {
	t.AddMember(email, newPatchsetRunners)
}

// LogPhase emits easy to recognize log like
// ===========================
// PHASE: ....
// ===========================
func (t *Test) LogPhase(ctx context.Context, format string, args ...any) {
	line := strings.Repeat("=", 80)
	format = fmt.Sprintf("\n%s\nPHASE: %s\n%s", line, format, line)
	logging.Debugf(ctx, format, args...)
}

// MakeCfgSingular return project config with a single ConfigGroup.
func MakeCfgSingular(cgName, gHost, gRepo, gRef string, builders ...*cfgpb.Verifiers_Tryjob_Builder) *cfgpb.Config {
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
				Verifiers: &cfgpb.Verifiers{
					GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
						CommitterList:            []string{committers},
						DryRunAccessList:         []string{dryRunners},
						NewPatchsetRunAccessList: []string{newPatchsetRunners},
					},
					Tryjob: &cfgpb.Verifiers_Tryjob{
						Builders: builders,
					},
				},
			},
		},
	}
}

// MakeCfgCombinable return project config with a combinable ConfigGroup.
func MakeCfgCombinable(cgName, gHost, gRepo, gRef string, builders ...*cfgpb.Verifiers_Tryjob_Builder) *cfgpb.Config {
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
				Verifiers: &cfgpb.Verifiers{
					GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
						CommitterList:    []string{committers},
						DryRunAccessList: []string{dryRunners},
					},
					Tryjob: &cfgpb.Verifiers_Tryjob{
						Builders: builders,
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
			Rand:                t.dsFlakinessRand,
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
			Rand:                             t.dsFlakinessRand,
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
	t.tqSweepChannel, err = dispatcher.NewChannel[struct{}](
		ctx,
		&dispatcher.Options[struct{}]{
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
		func(*buffer.Batch[struct{}]) error { return t.TQDispatcher.Sweep(ctx) },
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

// RunEndedPubSubTasks returns all the succeeded TQ tasks with RunEnded pubsub
// events.
func (t *Test) RunEndedPubSubTasks() tqtesting.TaskList {
	return t.SucceededTQTasks.Filter(func(t *tqtesting.Task) bool {
		_, ok := t.Payload.(*cvpubsub.PublishRunEndedTask)
		return ok
	})
}

func (t *Test) makeBuildbucketPubsub(ctx context.Context) (*pubsub.Topic, *pubsub.Subscription, func()) {
	srv := pstest.NewServer()
	client, err := pubsub.NewClient(ctx, t.Env.GAEInfo.CloudProject,
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		panic(err)
	}
	topic, err := client.CreateTopic(ctx, "bb-build")
	if err != nil {
		panic(err)
	}
	sub, err := client.CreateSubscription(ctx, bblistener.SubscriptionID, pubsub.SubscriptionConfig{Topic: topic})
	if err != nil {
		panic(err)
	}
	return topic, sub, func() {
		_ = client.Close()
		_ = srv.Close()
	}
}
