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

package cqdfake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
)

// StartingMessage is what CQDaemon fake posts when starting working on a Run.
//
// Yes, spelling is right -- this is what CQDaemon posted historically.
const StartingMessage = "CQ is trying da patch"

type CQDFake struct {
	LUCIProject string
	CV          migrationpb.MigrationServer
	GFake       *gf.Fake

	done chan struct{} // closed in Close() to stop the fake.
	wg   sync.WaitGroup
	m    sync.Mutex

	// attempts are active attempts indexed by attempt key.
	attempts map[string]*migrationpb.ReportedRun
	// atomic snapshot of keys after updateAttempts.
	latestAttemptsKeys atomic.Value

	candidatesClbk  atomic.Value
	verifyClbk      atomic.Value
	shouldCrashClbk atomic.Value

	cancelServeCtx func()
}

// Start starts CQDFake in background until the given context is cancelled or
// Close() is called.
func (cqd *CQDFake) Start(ctx context.Context) {
	cqd.m.Lock()
	defer cqd.m.Unlock()

	if cqd.done != nil {
		panic("called Start twice")
	}
	cqd.done = make(chan struct{})
	cqd.wg.Add(1)
	ctx, cqd.cancelServeCtx = context.WithCancel(ctx)
	go func() {
		defer cqd.wg.Done()
		defer cqd.cancelServeCtx()
		cqd.serve(ctx)
	}()
	// TODO(tandrii): add submitter thread fake.
}

// Close stops CQDaemon fake and waits for it to complete.
func (cqd *CQDFake) Close() {
	cqd.m.Lock()
	select {
	case <-cqd.done:
		// server told to stop already.
	default:
		// tell server to stop.
		close(cqd.done)
	}
	cqd.m.Unlock()

	// wait for stopping.
	cqd.wg.Wait()
}

// CandidatesClbk is called if CQDaemon is in charge to get Runs to work on.
//
// NOTE: the FetchExcludedCLs will still be applied on the output of the
// callback.
type CandidatesClbk func() []*migrationpb.ReportedRun

// SetCandidatesClbk (re)sets callback func called per CQDaemon loop if CQDaemon
// is in charge.
//
// Set it to mock what would-be candidates if Gerrit was queried directly.
// NOTE: the FetchExcludedCLs will still be applied on the output of the
// callback.
func (cqd *CQDFake) SetCandidatesClbk(clbk CandidatesClbk) {
	cqd.candidatesClbk.Store(clbk)
}

// VerifyClbk called once per CL per CQDaemon iteration.
//
// May modify the CL via copy-on-write and return the new value.
type VerifyClbk func(r *migrationpb.ReportedRun) *migrationpb.ReportedRun

// SetVerifyClbk (re)sets callback func called per CQDaemon active attempt
// once per loop.
func (cqd *CQDFake) SetVerifyClbk(clbk VerifyClbk) {
	cqd.verifyClbk.Store(clbk)
}

type Operation string

const (
	OpReportFinishedRun Operation = "OpReportFinishedRun"
)

// ShouldCrashClbk is consulted to decide if CQD should crash.
//
// key is attempt key if operation is specific to an Attempt.
type ShouldCrashClbk func(nextOp Operation, key string) bool

// SetShouldCrashClbk (re)sets callback func.
func (cqd *CQDFake) SetShouldCrashClbk(clbk ShouldCrashClbk) {
	cqd.shouldCrashClbk.Store(clbk)
}

////////////////////////////////////////////////////////////////////////////////
// State examiners.

// Returns sorted slice of attempt keys.
func (cqd *CQDFake) ActiveAttemptKeys() []string {
	v := cqd.latestAttemptsKeys.Load()
	if v == nil {
		return nil
	}
	return v.([]string)
}

////////////////////////////////////////////////////////////////////////////////
// Implementation.

func (cqd *CQDFake) serve(ctx context.Context) {
	timer := clock.NewTimer(clock.Tag(ctx, "cqdfake"))
	for {
		timer.Reset(10 * time.Second)
		select {
		case <-cqd.done:
			return
		case <-ctx.Done():
			return
		case <-timer.GetC():
			if err := cqd.iteration(ctx); err != nil {
				errors.Log(ctx, err)
			}
		}
	}
}

// iteration simulates one iteration of the CQDaemon loop.
func (cqd *CQDFake) iteration(ctx context.Context) error {
	if err := cqd.updateAttempts(ctx); err != nil {
		return err
	}
	if err := cqd.verifyAll(ctx); err != nil {
		return err
	}
	return nil
}

func (cqd *CQDFake) updateAttempts(ctx context.Context) error {
	candidates, err := cqd.fetchCandidates(ctx)
	if err != nil {
		return err
	}

	cqd.m.Lock()
	defer cqd.m.Unlock()

	seen := stringset.New(len(candidates))
	for _, candidate := range candidates {
		seen.Add(candidate.Attempt.Key)
	}

	var errs errors.MultiError
	for k, a := range cqd.attempts {
		if !seen.Has(k) {
			if err = cqd.logRunStatus(ctx, a); err == nil {
				err = cqd.deleteLocked(ctx, a)
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, candidate := range candidates {
		if _, exists := cqd.attempts[candidate.Attempt.Key]; !exists {
			if err := cqd.addLocked(ctx, candidate); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// Update attempt keys while we are holding the lock.
	cqd.setActiveAttemptKeysLocked()

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (cqd *CQDFake) fetchCandidates(ctx context.Context) ([]*migrationpb.ReportedRun, error) {
	req := migrationpb.FetchActiveRunsRequest{LuciProject: cqd.LUCIProject}
	resp, err := cqd.CV.FetchActiveRuns(cqd.migrationAPIContext(ctx), &req)
	if err != nil {
		return nil, err
	}
	out := make([]*migrationpb.ReportedRun, len(resp.GetActiveRuns()))
	for i, a := range resp.GetActiveRuns() {
		out[i] = &migrationpb.ReportedRun{
			Id: a.GetId(),
			Attempt: &cvbqpb.Attempt{
				Key:           common.RunID(a.GetId()).AttemptKey(),
				GerritChanges: make([]*cvbqpb.GerritChange, len(a.GetCls())),
				Status:        cvbqpb.AttemptStatus_STARTED,
			},
		}
		avail := availableClids(a)
		for j, cl := range a.GetCls() {
			out[i].Attempt.GerritChanges[j] = cl.GetGc()
			// Verify that each referenced deps is either a Run CL itself OR is in
			// FyiDeps.
			for _, d := range cl.GetDeps() {
				if _, yes := avail[d.GetId()]; !yes {
					panic(fmt.Errorf("ActiveRun %q: CL %d: references dep %d which isn't in given list %v",
						a.GetId(), cl.GetId(), d.GetId(), avail))
				}
			}
		}
	}
	return out, nil
}

func (cqd *CQDFake) verifyAll(ctx context.Context) error {
	cqd.m.Lock()
	defer func() {
		cqd.setActiveAttemptKeysLocked()
		cqd.m.Unlock()
	}()

	for k, before := range cqd.attempts {
		after := before
		cqd.m.Unlock()
		if f := cqd.verifyClbk.Load(); f != nil {
			after = f.(VerifyClbk)(before)
		}
		cqd.m.Lock()
		cqd.attempts[k] = after

		if after.Attempt.Status > cvbqpb.AttemptStatus_STARTED {
			if err := cqd.deleteLocked(ctx, after); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cqd *CQDFake) addLocked(ctx context.Context, r *migrationpb.ReportedRun) error {
	if cqd.attempts == nil {
		cqd.attempts = make(map[string]*migrationpb.ReportedRun, 1)
	}
	msg := fmt.Sprintf("Run %q | Attempt %q: %s", r.Id, r.Attempt.Key, StartingMessage)
	for _, cl := range r.Attempt.GerritChanges {
		req := &migrationpb.PostGerritMessageRequest{
			Project:    cqd.LUCIProject,
			AttemptKey: r.Attempt.Key,
			Host:       cl.Host,
			Change:     cl.Change,
			Revision:   "badly-faked-revision",
			Comment:    msg,
			SendEmail:  true,
		}
		if _, err := cqd.CV.PostGerritMessage(cqd.migrationAPIContext(ctx), req); err != nil {
			return err
		}
	}
	cqd.attempts[r.Attempt.Key] = proto.Clone(r).(*migrationpb.ReportedRun)
	logging.Debugf(ctx, "CQD: %s", msg)
	return nil
}

func (cqd *CQDFake) deleteLocked(ctx context.Context, r *migrationpb.ReportedRun) error {
	msg := fmt.Sprintf(
		"Run %q | Attempt %q finished with %s %s.",
		r.Id, r.Attempt.Key, r.Attempt.Status, r.Attempt.Substatus)

	if err := cqd.finalizeRunViaCV(ctx, r, msg); err != nil {
		return err
	}
	delete(cqd.attempts, r.Attempt.Key)
	logging.Debugf(ctx, "CQD: %s", msg)
	return nil
}

func (cqd *CQDFake) setActiveAttemptKeysLocked() {
	keys := make([]string, 0, len(cqd.attempts))
	for k := range cqd.attempts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cqd.latestAttemptsKeys.Store(keys)
}

func (cqd *CQDFake) finalizeRunViaCV(ctx context.Context, r *migrationpb.ReportedRun, msg string) error {
	req := &migrationpb.ReportVerifiedRunRequest{
		FinalMessage: msg,
		Run:          r,
	}
	switch r.Attempt.Status {
	case cvbqpb.AttemptStatus_ABORTED, cvbqpb.AttemptStatus_FAILURE, cvbqpb.AttemptStatus_INFRA_FAILURE:
		req.Action = migrationpb.ReportVerifiedRunRequest_ACTION_FAIL
	case cvbqpb.AttemptStatus_SUCCESS:
		if r.Attempt.GerritChanges[0].GetMode() == cvbqpb.Mode_FULL_RUN {
			req.Action = migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT
		} else {
			req.Action = migrationpb.ReportVerifiedRunRequest_ACTION_DRY_RUN_OK
		}
	}
	_, err := cqd.CV.ReportVerifiedRun(cqd.migrationAPIContext(ctx), req)
	return err
}

func (cqd *CQDFake) logRunStatus(ctx context.Context, r *migrationpb.ReportedRun) error {
	req := &migrationpb.FetchRunStatusRequest{
		AttemptKey:  r.GetAttempt().GetKey(),
		CvId:        r.GetId(),
		LuciProject: cqd.LUCIProject,
	}
	switch resp, err := cqd.CV.FetchRunStatus(cqd.migrationAPIContext(ctx), req); {
	case err != nil:
		logging.Errorf(ctx, "FetchRunStatus failed: %s", err)
		return err
	default:
		logging.Warningf(ctx, "RunStatus from CV: %s", resp)
		return nil
	}
}

func (cqd *CQDFake) migrationAPIContext(ctx context.Context) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:             identity.Identity("project:" + cqd.LUCIProject),
		PeerIdentityOverride: "user:cqdaemon@example.com",
	})
}

func (cqd *CQDFake) maybeCrash(ctx context.Context, next Operation, key string) error {
	f := cqd.shouldCrashClbk.Load()
	if f == nil || !(f.(ShouldCrashClbk)(next, key)) {
		return nil
	}
	logging.Debugf(ctx, "CQD was told to crash before %s %q", next, key)
	cqd.cancelServeCtx()
	return errors.New("CQD is crashing")
}

var cqdGerritUser = &gerritpb.AccountInfo{
	Email:     "cqdaemon@example.com",
	AccountId: 538183838,
}

func availableClids(a *migrationpb.ActiveRun) map[int64]struct{} {
	m := make(map[int64]struct{}, len(a.GetCls())+len(a.GetFyiDeps()))
	for _, cl := range a.GetCls() {
		m[cl.GetId()] = struct{}{}
	}
	for _, d := range a.GetFyiDeps() {
		m[d.GetId()] = struct{}{}
	}
	return m
}
