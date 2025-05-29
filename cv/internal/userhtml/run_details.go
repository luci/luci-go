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

package userhtml

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runquery"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func runDetails(c *router.Context) {
	ctx := c.Request.Context()

	rID := fmt.Sprintf("%s/%s", c.Params.ByName("Project"), c.Params.ByName("Run"))

	// Load the Run, checking its existence and ACLs.
	r, err := run.LoadRun(ctx, common.RunID(rID), acls.NewRunReadChecker())
	if err != nil {
		errPage(c, err)
		return
	}

	cls, latestTryjobs, logs, err := loadRunInfo(ctx, r)
	if err != nil {
		errPage(c, err)
		return
	}

	// Compute next and previous runs for all cls in parallel.
	clsAndLinks, err := computeCLsAndLinks(ctx, cls, r.ID)
	if err != nil {
		errPage(c, err)
		return
	}

	templates.MustRender(ctx, c.Writer, "pages/run_details.html", templates.Args{
		"Run":  r,
		"Logs": logs,
		"Cls":  clsAndLinks,
		"RelTime": func(ts time.Time) string {
			return humanize.RelTime(ts, startTime(ctx), "ago", "from now")
		},
		"LatestTryjobs": latestTryjobs,
	})
}

func loadRunInfo(ctx context.Context, r *run.Run) ([]*run.RunCL, []*uiTryjob, []*uiLogEntry, error) {
	var cls []*run.RunCL
	var latestTryjobs []*tryjob.Tryjob
	var runLogs []*run.LogEntry
	var tryjobLogs []*tryjob.ExecutionLogEntry
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		cls, err = run.LoadRunCLs(ctx, common.RunID(r.ID), r.CLs)
		if err != nil {
			return err
		}
		// Sort a stack of CLs by external ID.
		sort.Slice(cls, func(i, j int) bool { return cls[i].ExternalID < cls[j].ExternalID })
		return nil
	})
	eg.Go(func() (err error) {
		runLogs, err = run.LoadRunLogEntries(ctx, r.ID)
		return err
	})
	eg.Go(func() (err error) {
		tryjobLogs, err = tryjob.LoadExecutionLogs(ctx, r.ID)
		return err
	})
	eg.Go(func() (err error) {
		if len(r.Tryjobs.GetState().GetExecutions()) > 0 {
			// TODO(yiwzhang): backfill Tryjobs data reported from CQDaemon to
			// Tryjobs.State.Executions
			for _, execution := range r.Tryjobs.GetState().GetExecutions() {
				// TODO(yiwzhang): display the tryjob as not-started even if no
				// attempt has been triggered.
				if attempt := tryjob.LatestAttempt(execution); attempt != nil {
					latestTryjobs = append(latestTryjobs, &tryjob.Tryjob{
						ID: common.TryjobID(attempt.GetTryjobId()),
					})
				}
			}
			if err := datastore.Get(ctx, latestTryjobs); err != nil {
				return transient.Tag.Apply(errors.Fmt("failed to load tryjobs: %w", err))
			}
		} else {
			for _, tj := range r.Tryjobs.GetTryjobs() {
				launchedBy := r.ID
				if tj.Reused {
					launchedBy = ""
				}
				latestTryjobs = append(latestTryjobs, &tryjob.Tryjob{
					ExternalID: tryjob.ExternalID(tj.ExternalId),
					Definition: tj.Definition,
					Status:     tj.Status,
					Result:     tj.Result,
					LaunchedBy: launchedBy,
				})
			}
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, nil, err
	}
	uiLogEntries := make([]*uiLogEntry, len(runLogs)+len(tryjobLogs))
	for i, rl := range runLogs {
		uiLogEntries[i] = &uiLogEntry{
			runLog: rl,
			run:    r,
			cls:    cls,
		}
	}
	offset := len(runLogs)
	for i, tl := range tryjobLogs {
		uiLogEntries[offset+i] = &uiLogEntry{
			tryjobLog: tl,
			run:       r,
			cls:       cls,
		}
	}
	// ensure most recent log is at the top of the list.
	sort.Slice(uiLogEntries, func(i, j int) bool {
		return uiLogEntries[i].Time().After(uiLogEntries[j].Time())
	})
	return cls, makeUITryjobs(latestTryjobs, r.ID), uiLogEntries, nil
}

type clAndNeighborRuns struct {
	Prev, Next common.RunID
	CL         *run.RunCL
}

func (cl *clAndNeighborRuns) URLWithPatchset() string {
	return fmt.Sprintf("%s/%d", cl.CL.ExternalID.MustURL(), cl.CL.Detail.GetPatchset())
}

func (cl *clAndNeighborRuns) ShortWithPatchset() string {
	return fmt.Sprintf("%s/%d", displayCLExternalID(cl.CL.ExternalID), cl.CL.Detail.GetPatchset())
}

func computeCLsAndLinks(ctx context.Context, cls []*run.RunCL, rid common.RunID) ([]*clAndNeighborRuns, error) {
	ret := make([]*clAndNeighborRuns, len(cls))
	eg, ctx := errgroup.WithContext(ctx)
	for i, cl := range cls {
		eg.Go(func() error {
			prev, next, err := getNeighborsByCL(ctx, cl, rid)
			if err != nil {
				return errors.Fmt("unable to get previous and next runs for cl %s: %w", cl.ExternalID, err)
			}
			ret[i] = &clAndNeighborRuns{
				Prev: prev,
				Next: next,
				CL:   cl,
			}
			return nil
		})
	}
	return ret, eg.Wait()
}

func getNeighborsByCL(ctx context.Context, cl *run.RunCL, rID common.RunID) (common.RunID, common.RunID, error) {
	eg, ctx := errgroup.WithContext(ctx)

	prev := common.RunID("")
	eg.Go(func() error {
		qb := runquery.CLQueryBuilder{CLID: cl.ID, Limit: 1}.BeforeInProject(rID)
		switch keys, err := qb.GetAllRunKeys(ctx); {
		case err != nil:
			return err
		case len(keys) == 1:
			prev = common.RunID(keys[0].StringID())
			// It's OK to return the prev Run ID w/o checking ACLs because even if the
			// user can't see the prev Run, user is already served this Run's ID, which
			// contains the same LUCI project.
			if prev.LUCIProject() != rID.LUCIProject() {
				panic(fmt.Errorf("CLQueryBuilder.Before didn't limit project: %q vs %q", prev, rID))
			}
		}
		return nil
	})

	next := common.RunID("")
	eg.Go(func() error {
		// CLQueryBuilder gives Runs of the same project ordered from newest to
		// oldest. We need the oldest run which was created after the `rID`.
		// So, fetch all the Run IDs, and choose the last of them.
		//
		// In practice, there should be << 100 Runs per CL, but since we are
		// fetching just the keys and the query is very cheap, we go to 500.
		const limit = 500
		qb := runquery.CLQueryBuilder{CLID: cl.ID, Limit: limit}.AfterInProject(rID)
		switch keys, err := qb.GetAllRunKeys(ctx); {
		case err != nil:
			return err
		case len(keys) == limit:
			return errors.Fmt("too many Runs (>=%d) after %q", limit, rID)
		case len(keys) > 0:
			next = common.RunID(keys[len(keys)-1].StringID())
			// It's OK to return the next Run ID w/o checking ACLs because even if the
			// user can't see the next Run, user is already served this Run's ID, which
			// contains the same LUCI project.
			if next.LUCIProject() != rID.LUCIProject() {
				panic(fmt.Errorf("CLQueryBuilder.After didn't limit project: %q vs %q", next, rID))
			}
		}
		return nil
	})

	err := eg.Wait()
	return prev, next, err
}
