// Copyright 2020 The LUCI Authors.
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

package cvtesting

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	nativeDatastore "cloud.google.com/go/datastore"
	"github.com/golang/mock/gomock"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	serverauth "go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	bbfake "go.chromium.org/luci/cv/internal/buildbucket/fake"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/bq"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/common/tree/treetest"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

const gaeTopLevelDomain = ".appspot.com"

// TODO(tandrii): add fake config generation facilities.

// Test encapsulates typical setup for CV test.
//
// Typical use:
//
//	ct := cvtesting.Test{}
//	ctx := ct.SetUp(t)
type Test struct {
	TB testing.TB

	// Env simulates CV environment.
	Env *common.Env
	// GFake is a Gerrit fake. Defaults to an empty one.
	GFake *gf.Fake
	// BuildbucketFake is a Buildbucket fake. Defaults to an empty one.
	BuildbucketFake *bbfake.Fake
	// TreeFake is a fake Tree. Defaults to an open Tree.
	TreeFake *treetest.Fake
	// BQFake is a fake BQ client.
	BQFake *bq.Fake
	// TQDispatcher is a dispatcher with which task classes must be registered.
	//
	// Must not be set.
	TQDispatcher *tq.Dispatcher
	// TQ allows to run TQ tasks.
	TQ *tqtesting.Scheduler
	// SucceededTQTasks is a list of the TQ tasks that were executed successfully.
	SucceededTQTasks tqtesting.TaskList
	FailedTQTasks    tqtesting.TaskList

	// Clock allows to move time forward.
	// By default, the time is moved automatically is something waits on it.
	Clock testclock.TestClock
	// TSMonStore store keeps all metrics in memory and allows examination.
	TSMonStore store.Store

	// MaxDuration limits how long a test can run as a fail safe.
	//
	// Defaults to 10s to most likely finish in pre/post submit tests,
	// with limited CPU resources.
	// Set to ~10ms when debugging a hung test.
	MaxDuration time.Duration

	// GoMockCtl is the controller for gomock.
	GoMockCtl *gomock.Controller

	// authDB is used to mock CrIA memberships.
	authDB *authtest.FakeDB
}

type testingContextKeyType struct{}

// IsTestingContext checks if the given context was derived from one created by
// cvtesting.Test.SetUp().
func IsTestingContext(ctx context.Context) bool {
	return ctx.Value(testingContextKeyType{}) != nil
}

func (t *Test) SetUp(testingT testing.TB) context.Context {
	t.TB = testingT

	if t.Env == nil {
		t.Env = &common.Env{
			LogicalHostname: "luci-change-verifier" + gaeTopLevelDomain,
			HTTPAddressBase: "https://luci-change-verifier" + gaeTopLevelDomain,
			GAEInfo: struct {
				CloudProject string
				ServiceName  string
				InstanceID   string
			}{
				CloudProject: "luci-change-verifier",
				ServiceName:  "test-service",
				InstanceID:   "test-instance",
			},
		}
	}
	t.TB.Helper()

	// TODO - go.dev/issue/48157: use per-test case timeout once Golang provides
	// native support
	t.setMaxDuration()
	ctxShared := context.WithValue(context.Background(), testingContextKeyType{}, struct{}{})
	// Don't set the deadline (timeout) into the context given to the test,
	// as it may interfere with test clock.
	ctx, cancel := context.WithCancel(ctxShared)
	ctxTimed, cancelTimed := context.WithTimeout(ctxShared, t.MaxDuration)
	t.TB.Cleanup(func() {
		// Fail the test if the test has timed out.
		assert.That(t.TB, ctxTimed.Err(), should.ErrLike(nil), truth.LineContext())
		cancelTimed()
		cancel()
	})
	go func(ctx context.Context) {
		// Instead, watch for expiry of ctxTimed and cancel test `ctx`.
		select {
		case <-ctxTimed.Done():
			cancel()
		case <-ctx.Done():
			// Normal test termination.
			cancelTimed()
		}
	}(ctx)
	// setup the test clock first so that logger can use test clock timestamp.
	ctx = t.setUpTestClock(ctx, t.TB)
	if testing.Verbose() {
		// TODO(crbug/1282023): make this logger emit testclock-based timestamps.
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
	}
	// setup timerCallback after setup logger so that the logging in the
	// callback function can honor the verbose mode.
	ctx = t.setTestClockTimerCB(ctx)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	// TQDispatcher must not be set
	assert.Loosely(t.TB, t.TQDispatcher, should.BeNil, truth.LineContext())
	t.TQDispatcher = &tq.Dispatcher{}
	ctx, t.TQ = tq.TestingContext(ctx, t.TQDispatcher)
	t.TQ.TaskSucceeded = tqtesting.TasksCollector(&t.SucceededTQTasks)
	t.TQ.TaskFailed = tqtesting.TasksCollector(&t.FailedTQTasks)

	if t.GFake == nil {
		t.GFake = &gf.Fake{}
	}
	if t.BuildbucketFake == nil {
		t.BuildbucketFake = &bbfake.Fake{}
	}
	if t.TreeFake == nil {
		t.TreeFake = treetest.NewFake(ctx, tree.Open)
	}
	if t.BQFake == nil {
		t.BQFake = &bq.Fake{}
	}

	ctx = t.installDS(ctx, t.TB)
	ctx = txndefer.FilterRDS(ctx)
	t.authDB = authtest.NewFakeDB()
	ctx = serverauth.WithState(ctx, &authtest.FakeState{FakeDB: t.authDB})

	ctx, _, _ = tsmon.WithFakes(ctx)
	t.TSMonStore = store.NewInMemory(&target.Task{})
	tsmon.GetState(ctx).SetStore(t.TSMonStore)

	t.GoMockCtl = gomock.NewController(t.TB)
	assert.That(t.TB, srvcfg.SetTestListenerConfig(ctx, &listenerpb.Settings{}, nil), should.ErrLike(nil), truth.LineContext())

	return ctx
}

func (t *Test) RoundTestClock(multiple time.Duration) {
	t.Clock.Set(t.Clock.Now().Add(multiple).Truncate(multiple))
}

func (t *Test) GFactory() gerrit.Factory {
	return gerrit.CachingFactory(16, gerrit.TimeLimitedFactory(gerrit.InstrumentedFactory(t.GFake)))
}

// TSMonSentValue returns the latest value of the given metric.
//
// If not set, returns nil.
func (t *Test) TSMonSentValue(ctx context.Context, m types.Metric, fieldVals ...any) any {
	resetTime := time.Time{}
	return t.TSMonStore.Get(ctx, m, resetTime, fieldVals)
}

// TSMonSentDistr returns the latest distr value of the given metric.
//
// If not set, returns nil.
// Panics if metric's value is not a distribution.
func (t *Test) TSMonSentDistr(ctx context.Context, m types.Metric, fieldVals ...any) *distribution.Distribution {
	v := t.TSMonSentValue(ctx, m, fieldVals...)
	if v == nil {
		return nil
	}
	d, ok := v.(*distribution.Distribution)
	if !ok {
		panic(fmt.Errorf("metric %q value is not a %T, but %T", m.Info().Name, d, v))
	}
	return d
}

func (t *Test) setMaxDuration() {
	// Can't use Go's test timeout because it is per TestXYZ func,
	// which typically instantiates & runs several `cvtesting.Test`s.
	switch s := os.Getenv("CV_TEST_TIMEOUT_SEC"); {
	case s != "":
		v, err := strconv.ParseInt(s, 10, 31)
		if err != nil {
			panic(err)
		}
		t.MaxDuration = time.Duration(v) * time.Second
	case t.MaxDuration != time.Duration(0):
		// TODO(tandrii): remove the possibility to override this per test in favor
		// of CV_TEST_TIMEOUT_SEC env var.
	case raceDetectionEnabled:
		t.MaxDuration = 90 * time.Second
	default:
		t.MaxDuration = 20 * time.Second
	}
}

// DisableProjectInGerritListener updates the cached config to disable LUCI
// projects matching a given regexp in Listener.
func (t *Test) DisableProjectInGerritListener(ctx context.Context, projectRE string) {
	cfg, err := srvcfg.GetListenerConfig(ctx, nil)
	if err != nil {
		panic(err)
	}
	existing := stringset.NewFromSlice(cfg.DisabledProjectRegexps...)
	existing.Add(projectRE)
	cfg.DisabledProjectRegexps = existing.ToSortedSlice()
	if err := srvcfg.SetTestListenerConfig(ctx, cfg, nil); err != nil {
		panic(err)
	}
}

func (t *Test) installDS(ctx context.Context, testingT testing.TB) context.Context {
	assert.That(testingT, strings.HasSuffix(t.Env.LogicalHostname, gaeTopLevelDomain), should.BeTrue)
	appID := t.Env.LogicalHostname[:len(t.Env.LogicalHostname)-len(gaeTopLevelDomain)]

	if ctx, ok := t.installDSReal(ctx, testingT); ok {
		return memory.UseInfo(ctx, appID)
	}
	if ctx, ok := t.installDSEmulator(ctx, testingT); ok {
		return memory.UseInfo(ctx, appID)
	}

	ctx = memory.UseWithAppID(ctx, appID)
	// CV runs against Firestore backend, which is consistent.
	datastore.GetTestable(ctx).Consistent(true)
	// Intentionally not enabling AutoIndex so that new code accidentally needing
	// a new index adds it both here (for the rest of CV tests to work, notably
	// e2e ones) and into appengine/index.yaml.
	datastore.GetTestable(ctx).AutoIndex(false)
	return ctx
}

// installDSProd configures CV tests to run with actual DS.
//
// If DATASTORE_PROJECT ENV var isn't set, returns false.
//
// To use, first
//
//	$ luci-auth context -- bash
//	$ export DATASTORE_PROJECT=my-cloud-project-with-datastore
//
// and then run go tests the usual way, e.g.:
//
//	$ go test ./...
func (t *Test) installDSReal(ctx context.Context, testingT testing.TB) (context.Context, bool) {
	project := os.Getenv("DATASTORE_PROJECT")
	if project == "" {
		return ctx, false
	}
	if project == "luci-change-verifier" {
		panic("Don't use production CV project. Using -dev is OK.")
	}

	at := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
		Scopes: serverauth.CloudOAuthScopes,
	})
	ts, err := at.TokenSource()
	if err != nil {
		err = errors.Annotate(err, "failed to initialize the token source (are you in `$ luci-auth context`?)").Err()
		assert.That(testingT, err, should.ErrLike(nil))
	}

	logging.Debugf(ctx, "Using DS of project %q", project)
	client, err := nativeDatastore.NewClient(ctx, project, option.WithTokenSource(ts))
	assert.That(testingT, err, should.ErrLike(nil))
	return t.installDSshared(ctx, testingT, project, client), true
}

// installDSEmulator configures CV tests to run with DS emulator.
//
// If DATASTORE_EMULATOR_HOST ENV var isn't set, returns false.
//
// To use, run
//
//	$ gcloud beta emulators datastore start --consistency=1.0
//
// and export DATASTORE_EMULATOR_HOST as printed by above command.
//
// NOTE: as of Feb 2021, emulator runs in legacy Datastore mode,
// not Firestore.
func (t *Test) installDSEmulator(ctx context.Context, testingT testing.TB) (context.Context, bool) {
	emulatorHost := os.Getenv("DATASTORE_EMULATOR_HOST")
	if emulatorHost == "" {
		return ctx, false
	}

	logging.Debugf(ctx, "Using DS emulator at %q", emulatorHost)
	client, err := nativeDatastore.NewClient(ctx, "luci-gae-emulator-test")
	assert.That(testingT, err, should.ErrLike(nil))
	return t.installDSshared(ctx, testingT, "luci-gae-emulator-test", client), true
}

func (t *Test) installDSshared(ctx context.Context, testingT testing.TB, cloudProject string, client *nativeDatastore.Client) context.Context {
	testingT.Cleanup(func() { _ = client.Close() })
	ctx = (&cloud.ConfigLite{ProjectID: cloudProject, DS: client}).Use(ctx)
	maybeCleanupOldDSNamespaces(ctx)

	// Enter a namespace for this tests.
	ns := genDSNamespaceName(time.Now())
	logging.Debugf(ctx, "Using %q DS namespace", ns)
	ctx = info.MustNamespace(ctx, ns)
	// Failure to clear is hard before the test,
	// ignored after the test.
	assert.That(testingT, clearDS(ctx), should.ErrLike(nil))
	testingT.Cleanup(func() { _ = clearDS(ctx) })
	return ctx
}

func genDSNamespaceName(t time.Time) string {
	rnd := make([]byte, 8)
	if _, err := cryptorand.Read(rnd); err != nil {
		panic(err)
	}
	return fmt.Sprintf("testing-%s-%s", t.Format("2006-01-02"), hex.EncodeToString(rnd))
}

var dsNamespaceRegexp = regexp.MustCompile(`^testing-(\d{4}-\d\d-\d\d)-[0-9a-f]+$`)

func isOldTestDSNamespace(ns string, now time.Time) bool {
	m := dsNamespaceRegexp.FindSubmatch([]byte(ns))
	if len(m) == 0 {
		return false
	}
	// Anything up ~2 days old should be kept to avoid accidentally removing
	// currently under test namespace in presence of timezones and out of sync
	// clocks.
	const maxAge = 2 * 24 * time.Hour
	t, err := time.Parse("2006-01-02", string(m[1]))
	if err != nil {
		panic(err)
	}
	return now.Sub(t) > maxAge
}

func clearDS(ctx context.Context) error {
	// Execute a kindless query to clear entire namespace.
	q := datastore.NewQuery("").KeysOnly(true)
	var allKeys []*datastore.Key
	if err := datastore.GetAll(ctx, q, &allKeys); err != nil {
		return errors.Annotate(err, "failed to get entities").Err()
	}
	if err := datastore.Delete(ctx, allKeys); err != nil {
		return errors.Annotate(err, "failed to delete %d entities", len(allKeys)).Err()
	}
	return nil
}

func maybeCleanupOldDSNamespaces(ctx context.Context) {
	if rand.Intn(1024) < 1020 { // ~99% of cases.
		return
	}
	q := datastore.NewQuery("__namespace__").KeysOnly(true)
	var allKeys []*datastore.Key
	if err := datastore.GetAll(ctx, q, &allKeys); err != nil {
		logging.Warningf(ctx, "failed to query all namespaces: %s", err)
		return
	}
	now := time.Now()
	var toDelete []string
	for _, k := range allKeys {
		ns := k.StringID()
		if isOldTestDSNamespace(ns, now) {
			toDelete = append(toDelete, ns)
		}
	}
	logging.Debugf(ctx, "cleaning up %d old namespaces", len(toDelete))
	for _, ns := range toDelete {
		logging.Debugf(ctx, "cleaning up %s", ns)
		if err := clearDS(info.MustNamespace(ctx, ns)); err != nil {
			logging.Errorf(ctx, "failed to clean old DS namespace %s: %s", ns, err)
		}
	}
}

// setUpTestClock simulates passage of time w/o idling CPU.
func (t *Test) setUpTestClock(ctx context.Context, testingT testing.TB) context.Context {
	if t.Clock != nil {
		return clock.Set(ctx, t.Clock)
	}
	// Use a date-time that is easy to eyeball in logs.
	utc := time.Date(2020, time.February, 2, 10, 30, 00, 0, time.UTC)
	// But set it up in a clock as a local time to expose incorrect assumptions of UTC.
	now := time.Date(2020, time.February, 2, 13, 30, 00, 0, time.FixedZone("Fake local", 3*60*60))
	assert.That(testingT, now.Equal(utc), should.BeTrue)
	ctx, t.Clock = testclock.UseTime(ctx, now)
	return ctx
}

// setTestClockTimerCB moves test time forward if something we recognize waits
// for it.
func (t *Test) setTestClockTimerCB(ctx context.Context) context.Context {
	// Testclock calls this callback every time something is waiting.
	// To avoid getting stuck tests, we need to move testclock forward by the
	// requested duration in most cases but not all.
	moveIf := stringset.NewFromSlice(
		// Used by tqtesting to wait until ETA of the next task.
		tqtesting.ClockTag,
		// Used to retry on outgoing requests for BB and Gerrit.
		common.LaunchRetryClockTag,
	)
	ignoreIf := stringset.NewFromSlice(
		// Used in clock.WithTimeout(ctx) | clock.WithDeadline(ctx).
		clock.ContextDeadlineTag,
	)
	t.Clock.SetTimerCallback(func(dur time.Duration, timer clock.Timer) {
		tags := testclock.GetTags(timer)
		move, ignore := 0, 0
		for _, tag := range tags {
			switch {
			case moveIf.Has(tag):
				move++
			case ignoreIf.Has(tag):
				ignore++
			default:
				// Ignore by default, but log it to help fix the test if it gets stuck.
				logging.Warningf(ctx, "ignoring unexpected timer tag: %q. If test is stuck, add tag to `moveIf` above this log line", tag)
			}
		}
		// In ~all cases, there is exactly 1 tag, but be future proof.
		switch {
		case move > 0:
			logging.Debugf(ctx, "moving test clock %s by %s forward for %s", t.Clock.Now(), dur, tags)
			t.Clock.Add(dur)
		case ignore == 0:
			logging.Warningf(ctx, "ignoring timer without tags. If test is stuck, tag the waits via `clock` library")
		}
	})
	return ctx
}

// AddMember adds a given member into a given luci auth group.
//
// The email may omit domain. In that case, this method will add "@example.com"
// as the domain name.
func (t *Test) AddMember(email, group string) {
	if _, err := mail.ParseAddress(email); err != nil {
		email = fmt.Sprintf("%s@example.com", email)
	}
	id, err := identity.MakeIdentity(fmt.Sprintf("user:%s", email))
	if err != nil {
		panic(err)
	}
	t.authDB.AddMocks(authtest.MockMembership(id, group))
}

// AddPermission grants permission to the member in the given realm.
//
// The email may omit domain. In that case, this method will add "@example.com"
// as the domain name.
func (t *Test) AddPermission(email string, perm realms.Permission, realm string) {
	if _, err := mail.ParseAddress(email); err != nil {
		email = fmt.Sprintf("%s@example.com", email)
	}
	id, err := identity.MakeIdentity(fmt.Sprintf("user:%s", email))
	if err != nil {
		panic(err)
	}
	t.authDB.AddMocks(authtest.MockPermission(id, realm, perm))
}

func (t *Test) ResetMockedAuthDB(ctx context.Context) {
	t.authDB = authtest.NewFakeDB()
	serverauth.GetState(ctx).(*authtest.FakeState).FakeDB = t.authDB
}
