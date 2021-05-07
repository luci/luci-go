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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/protobuf/proto"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/migration/migrationcfg"
)

const ClockTag = "cqdfake"

type CQDFake struct {
	LUCIProject string
	CV          migrationpb.MigrationServer
	GFake       *gf.Fake

	done chan struct{} // closed in Close() to stop the fake.
	wg   sync.WaitGroup
	m    sync.Mutex

	// attempts are active attempts indexed by attempt key.
	attempts map[string]*migrationpb.Run

	candidatesClbk atomic.Value
	verifyClbk     atomic.Value
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
	go func() {
		defer cqd.wg.Done()
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
type CandidatesClbk func() []*migrationpb.Run

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
type VerifyClbk func(r *migrationpb.Run, cvInCharge bool) *migrationpb.Run

// SetVerifyClbk (re)sets callback func called per CQDaemon active attempt
// once per loop.
func (cqd *CQDFake) SetVerifyClbk(clbk VerifyClbk) {
	cqd.verifyClbk.Store(clbk)
}

////////////////////////////////////////////////////////////////////////////////
// State examiners.

// Returns sorted slice of attempt keys.
func (cqd *CQDFake) ActiveAttemptKeys() []string {
	cqd.m.Lock()
	defer cqd.m.Unlock()
	out := make([]string, 0, len(cqd.attempts))
	for k := range cqd.attempts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

////////////////////////////////////////////////////////////////////////////////
// Implementation.

func (cqd *CQDFake) serve(ctx context.Context) {
	timer := clock.NewTimer(clock.Tag(ctx, ClockTag))
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
	cvInCharge, err := migrationcfg.IsCQDUsingMyRuns(ctx, cqd.LUCIProject)
	if err != nil {
		return err
	}
	if err := cqd.updateAttempts(ctx, cvInCharge); err != nil {
		return err
	}
	if err := cqd.verifyAll(ctx, cvInCharge); err != nil {
		return err
	}
	return nil
}

func (cqd *CQDFake) updateAttempts(ctx context.Context, cvInCharge bool) error {
	candidates, err := cqd.fetchCandidates(ctx, cvInCharge)
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
			if err := cqd.deleteLocked(ctx, a, cvInCharge); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, candidate := range candidates {
		if _, exists := cqd.attempts[candidate.Attempt.Key]; !exists {
			if err := cqd.addLocked(ctx, candidate, cvInCharge); err != nil {
				errs = append(errs, err)
			}
		}
	}
	// TODO(tandrii): notify CV about active attempts.
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (cqd *CQDFake) fetchCandidates(ctx context.Context, cvInCharge bool) ([]*migrationpb.Run, error) {
	if cvInCharge {
		req := migrationpb.FetchActiveRunsRequest{LuciProject: cqd.LUCIProject}
		resp, err := cqd.CV.FetchActiveRuns(cqd.migrationApiContext(ctx), &req)
		return resp.GetRuns(), err
	}

	f := cqd.candidatesClbk.Load()
	if f == nil {
		logging.Warningf(ctx, "CQDaemon active, but no candaidate callback set. Forgot to call CQDFake.SetCandidatesClbk?")
		return nil, nil
	}
	runs := f.(CandidatesClbk)()
	if len(runs) == 0 {
		return nil, nil
	}

	// Filter out all runs with CLs matching those which CV is still processing.
	req := &migrationpb.FetchExcludedCLsRequest{LuciProject: cqd.LUCIProject}
	exCls, err := cqd.CV.FetchExcludedCLs(cqd.migrationApiContext(ctx), req)
	if err != nil {
		return nil, err
	}
	clKey := func(cl *cvbqpb.GerritChange) string {
		return fmt.Sprintf("%s/%d", cl.GetHost(), cl.GetChange())
	}
	exMap := stringset.New(len(exCls.GetCls()))
	for _, cl := range exCls.GetCls() {
		exMap.Add(clKey(cl))
	}
	out := runs[:0]
	for _, r := range runs {
		skip := false
		for _, cl := range r.GetCls() {
			if exMap.Has(clKey(cl.GetGc())) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, r)
		}
	}
	return out, nil
}

func (cqd *CQDFake) verifyAll(ctx context.Context, cvInCharge bool) error {
	cqd.m.Lock()
	for k, before := range cqd.attempts {
		after := before
		cqd.m.Unlock()
		if f := cqd.verifyClbk.Load(); f != nil {
			after = f.(VerifyClbk)(before, cvInCharge)
		}
		cqd.m.Lock()
		cqd.attempts[k] = after
		// TODO(tandrii): act upon after.Status.
	}
	cqd.m.Unlock()
	return nil
}

func (cqd *CQDFake) addLocked(ctx context.Context, r *migrationpb.Run, cvInCharge bool) error {
	// TODO(tandrii): post Gerrit comment.
	if cqd.attempts == nil {
		cqd.attempts = make(map[string]*migrationpb.Run, 1)
	}
	cqd.attempts[r.Attempt.Key] = proto.Clone(r).(*migrationpb.Run)
	return nil
}

func (cqd *CQDFake) deleteLocked(ctx context.Context, r *migrationpb.Run, cvInCharge bool) error {
	delete(cqd.attempts, r.Attempt.Key)
	// TODO(tandrii): post Gerrit comment and/or notify CV.
	return nil
}

func (cqd *CQDFake) migrationApiContext(ctx context.Context) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:             identity.Identity("project:" + cqd.LUCIProject),
		PeerIdentityOverride: "user:cqdaemon@example.com",
	})
}
