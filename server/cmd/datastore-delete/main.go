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

// Executable datastore-delete deletes all data of a specified kind in a
// Datastore database.
//
// First run in a dry run mode to see how many entities will be deleted:
//
//	go run main.go -cloud-project <project-id>
//
// Then run for real:
//
//	go run main.go -cloud-project <project-id> -delete
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
)

var (
	cloudProject = flag.String("cloud-project", "", "Cloud Datastore cloud project")
	kind         = flag.String("kind", "", "Datastore Kind to delete all entries for")
	delete       = flag.Bool("delete", false, "If set, actually delete the data")
	workers      = flag.Int("workers", 256, "Number of goroutines doing deletions")
)

func main() {
	flag.Parse()
	if *cloudProject == "" {
		fmt.Fprintf(os.Stderr, "-cloud-project is required\n")
		os.Exit(2)
	}

	if *kind == "" {
		fmt.Fprintf(os.Stderr, "-kind is required\n")
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
	keys := make(chan *datastore.Key, 50000)
	visitor := visitor{
		now:        clock.Now(ctx).UTC(),
		delete:     *delete,
		nextReport: clock.Now(ctx).Add(time.Second),
	}

	// A goroutine pool to process visited entities.
	gr, gctx := errgroup.WithContext(ctx)
	for i := 0; i < *workers; i++ {
		gr.Go(func() error {
			for s := range keys {
				visitor.process(gctx, s)
				visitor.reportMaybe(gctx)
			}
			return nil
		})
	}

	// A mapper that feeds entities to the visitor goroutine pool.
	logging.Infof(ctx, "Visiting %s entities...", *kind)
	mapErr := dsmapperlite.Map(ctx, datastore.NewQuery(*kind).KeysOnly(true), 32, 1000,
		func(ctx context.Context, _ int, key *datastore.Key) error {
			visitor.visit(ctx, key)
			keys <- key
			visitor.reportMaybe(ctx)
			return nil
		},
	)
	close(keys)
	visitor.visitedAll(ctx)
	grErr := gr.Wait()

	visitor.report(ctx, true)

	if grErr != nil {
		return errors.Fmt("when processing %s: %w", *kind, grErr)
	}
	if mapErr != nil {
		return errors.Fmt("when visiting %s: %w", *kind, mapErr)
	}
	return nil
}

type visitor struct {
	now    time.Time
	delete bool

	m sync.Mutex

	visited int // total number of entities visited

	pendingDelete int // entities queued for delete

	deleted int // total number of successfully deleted entities
	errors  int // total number of deletion errors

	reportM      sync.Mutex
	nextReport   time.Time // when to print the next progress report
	doneVisiting bool      // true if done visiting, but still processing
}

// visit returns true if a key needs to be processed.
func (v *visitor) visit(ctx context.Context, s *datastore.Key) {
	v.m.Lock()
	defer v.m.Unlock()

	v.visited++
}

// process deletes a key.
func (v *visitor) process(ctx context.Context, s *datastore.Key) {
	var err error
	if v.delete {
		if err = deleteKey(ctx, s); err != nil {
			logging.Errorf(ctx, "%s: %s", s, err)
		}
	}

	v.m.Lock()
	defer v.m.Unlock()

	v.pendingDelete--
	if v.delete {
		if err != nil {
			v.errors++
		} else {
			v.deleted++
		}
	}
}

// visitedAll is called when all keys are visited.
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
	logging.Infof(ctx, "Entities pending delete by the tool:      %d", v.pendingDelete)
	logging.Infof(ctx, "Successfully deleted entities:            %d", v.deleted)
	logging.Infof(ctx, "Update errors:                            %d", v.errors)
	logging.Infof(ctx, "-------------------------------------------")
}

func deleteKey(ctx context.Context, key *datastore.Key) error {
	return datastore.Delete(ctx, key)
}
