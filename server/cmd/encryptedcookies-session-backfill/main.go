// Copyright 2023 The LUCI Authors.
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

// Executable encryptedcookies-session-backfill backfills ExpireAt field in
// session datastore entities used by encryptedcookies module.
//
// First run in a dry run mode to see how many entities will be updated:
//
//	go run main.go -cloud-project <project-id>
//
// Then run for real:
//
//	go run main.go -cloud-project <project-id> -rewrite
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	cloudds "cloud.google.com/go/datastore"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/server/dsmapper/dsmapperlite"
	dssession "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

var (
	cloudProject = flag.String("cloud-project", "", "Cloud Datastore cloud project")
	rewrite      = flag.Bool("rewrite", false, "If set, overwrite ExpiryAt if it is missing or invalid")
	workers      = flag.Int("workers", 256, "Number of goroutines doing rewrites")
)

func main() {
	flag.Parse()
	if *cloudProject == "" {
		fmt.Fprintf(os.Stderr, "-cloud-project is required\n")
		os.Exit(2)
	}

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithCancel(ctx)
	signals.HandleInterrupt(cancel)

	if err := run(ctx); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	scopes := []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	}

	ts, err := auth.NewAuthenticator(ctx, auth.SilentLogin, chromeinfra.SetDefaultAuthOptions(auth.Options{
		Scopes: scopes,
	})).TokenSource()
	switch {
	case err == auth.ErrLoginRequired:
		return errors.Fmt("Need to login. Run `luci-auth login -scopes \"%s\"`", strings.Join(scopes, " "))
	case err != nil:
		return errors.Fmt("failed to get token source: %w", err)
	}

	client, err := cloudds.NewClient(ctx, *cloudProject,
		option.WithTokenSource(ts),
		option.WithGRPCConnectionPool(*workers/16),
	)
	if err != nil {
		return errors.Fmt("failed to instantiate the datastore client: %w", err)
	}

	ctx = (&cloud.ConfigLite{
		ProjectID: *cloudProject,
		DS:        client,
	}).Use(ctx)

	return reallyRun(ctx)
}

func reallyRun(ctx context.Context) error {
	sessions := make(chan *dssession.SessionEntity, 50000)
	visitor := visitor{
		now:        clock.Now(ctx).UTC(),
		rewrite:    *rewrite,
		nextReport: clock.Now(ctx).Add(time.Second),
	}

	// A goroutine pool to process visited entities.
	gr, gctx := errgroup.WithContext(ctx)
	for i := 0; i < *workers; i++ {
		gr.Go(func() error {
			for s := range sessions {
				visitor.process(gctx, s)
				visitor.reportMaybe(gctx)
			}
			return nil
		})
	}

	// A mapper that feeds entities to the visitor goroutine pool.
	logging.Infof(ctx, "Visiting Session entities...")
	mapErr := dsmapperlite.Map(ctx, datastore.NewQuery("encryptedcookies.Session"), 32, 1000,
		func(ctx context.Context, _ int, s *dssession.SessionEntity) error {
			if visitor.visit(ctx, s) {
				sessions <- s
			}
			visitor.reportMaybe(ctx)
			return nil
		},
	)
	close(sessions)
	visitor.visitedAll(ctx)
	grErr := gr.Wait()

	visitor.report(ctx, true)

	if grErr != nil {
		return errors.Fmt("when processing SessionEntity: %w", grErr)
	}
	if mapErr != nil {
		return errors.Fmt("when visiting SessionEntity: %w", mapErr)
	}
	return nil
}

type visitor struct {
	now     time.Time
	rewrite bool

	m sync.Mutex

	visited       int // total number of entities visited
	noExpiry      int // entities without ExpireAt field
	expiryValid   int // entities with correct ExpireAt
	expiryInvalid int // entities with present, but invalid ExpireAt
	expiryUnknown int // entities with unpopulated LastRefresh

	expiredForReal     int // expired entities with ExpireAt set
	freshForReal       int // fresh entities with ExpireAt set
	expiredWhenUpdated int // expired entities without ExpireAt set yet
	freshWhenUpdated   int // fresh entities without ExpireAt set yet

	pendingRewrite int // entities queued for rewrite

	rewritten int // total number of successfully updated entities
	errors    int // total number of update errors

	reportM      sync.Mutex
	nextReport   time.Time // when to print the next progress report
	doneVisiting bool      // true if done visiting, but still processing
}

// visit returns true if a session needs to be processed.
func (v *visitor) visit(ctx context.Context, s *dssession.SessionEntity) bool {
	v.m.Lock()
	defer v.m.Unlock()

	v.visited++

	var expectedExpiry time.Time
	if s.Session.GetLastRefresh() != nil {
		expectedExpiry = expectedExpiryAt(s)
	}

	if s.ExpireAt.IsZero() {
		v.noExpiry++
	}

	if expectedExpiry.IsZero() {
		v.expiryUnknown++
	}

	if !s.ExpireAt.IsZero() && !expectedExpiry.IsZero() {
		if s.ExpireAt.Equal(expectedExpiry) {
			v.expiryValid++
			if v.now.After(s.ExpireAt) {
				v.expiredForReal++
			} else {
				v.freshForReal++
			}
		} else {
			v.expiryInvalid++
		}
	}

	needRewrite := !expectedExpiry.IsZero() && !s.ExpireAt.Equal(expectedExpiry)
	if needRewrite {
		v.pendingRewrite++
		if v.now.After(expectedExpiry) {
			v.expiredWhenUpdated++
		} else {
			v.freshWhenUpdated++
		}
	}

	return needRewrite
}

// process updates a session.
func (v *visitor) process(ctx context.Context, s *dssession.SessionEntity) {
	var err error
	if v.rewrite {
		if err = updateExpiryAt(ctx, s.ID); err != nil {
			logging.Errorf(ctx, "%s: %s", s.ID, err)
		}
	}

	v.m.Lock()
	defer v.m.Unlock()

	v.pendingRewrite--
	if v.rewrite {
		if err != nil {
			v.errors++
		} else {
			v.rewritten++
		}
	}
}

// visitedAll is called when all sessions are visited.
func (v *visitor) visitedAll(ctx context.Context) {
	v.reportM.Lock()
	v.doneVisiting = true
	v.reportM.Unlock()
	v.report(ctx, true)
}

// reportMaybe prints a progress report if it is time.
func (v *visitor) reportMaybe(ctx context.Context) {
	now := clock.Now(ctx)

	v.reportM.Lock()
	needReport := now.After(v.nextReport)
	if needReport {
		v.nextReport = now.Add(time.Second)
	}
	doneVisiting := v.doneVisiting
	v.reportM.Unlock()

	if needReport {
		v.report(ctx, doneVisiting)
	}
}

// report prints a progress report.
func (v *visitor) report(ctx context.Context, doneVisiting bool) {
	v.m.Lock()
	defer v.m.Unlock()

	logging.Infof(ctx, "-------------------------------------------")
	if doneVisiting {
		logging.Infof(ctx, "All visited entities:                     %d", v.visited)
	} else {
		logging.Infof(ctx, "Entities visited so far:                  %d", v.visited)
	}
	logging.Infof(ctx, "Entities without ExpiryAt:                %d", v.noExpiry)
	logging.Infof(ctx, "Entities with valid ExpiryAt:             %d", v.expiryValid)
	logging.Infof(ctx, "Entities with invalid ExpiryAt:           %d", v.expiryInvalid)
	logging.Infof(ctx, "Entities already pending TTL cleanup:     %d", v.expiredForReal)
	logging.Infof(ctx, "Entities that aren't pending TTL cleanup: %d", v.freshForReal)
	logging.Infof(ctx, "Entities to become eligible for cleanup:  %d", v.expiredWhenUpdated)
	logging.Infof(ctx, "Entities to become fresh after rewrite:   %d", v.freshWhenUpdated)
	logging.Infof(ctx, "Entities pending rewrite by the tool:     %d", v.pendingRewrite)
	logging.Infof(ctx, "Successfully updated entities:            %d", v.rewritten)
	logging.Infof(ctx, "Update errors:                            %d", v.errors)
	logging.Infof(ctx, "-------------------------------------------")
}

func expectedExpiryAt(s *dssession.SessionEntity) time.Time {
	if s.Session.LastRefresh == nil {
		panic("LastRefresh must be populated")
	}
	return s.Session.LastRefresh.AsTime().
		Add(dssession.InactiveSessionExpiration).
		Round(time.Microsecond). // datastore rounds timestamps to microseconds
		UTC()
}

func updateExpiryAt(ctx context.Context, sid string) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		s := &dssession.SessionEntity{ID: sid}
		if err := datastore.Get(ctx, s); err != nil {
			return err
		}
		if s.Session.LastRefresh == nil {
			return errors.New("field LastRefresh is suddenly nil")
		}
		expected := expectedExpiryAt(s)
		if s.ExpireAt.Equal(expected) {
			return nil
		}
		s.ExpireAt = expected
		return datastore.Put(ctx, s)
	}, nil)
}
