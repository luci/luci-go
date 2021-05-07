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
	"sync"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

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
	// TODO(tandrii): implement updating cqd.attempts based on fetchCandidates().
	return nil
}

func (cqd *CQDFake) verifyAll(ctx context.Context, cvInCharge bool) error {
	cqd.m.Lock()
	for k, before := range cqd.attempts {
		after := before
		cqd.m.Unlock()
		if f := cqd.verifyClbk.Load().(VerifyClbk); f != nil {
			after = f(before, cvInCharge)
		}
		cqd.m.Lock()
		cqd.attempts[k] = after
		// TODO(tandrii): act upon after.Status.
	}
	cqd.m.Unlock()
	return nil
}
