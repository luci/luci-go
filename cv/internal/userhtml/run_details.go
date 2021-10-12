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
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func runDetails(c *router.Context) {
	ctx := c.Context

	rID := fmt.Sprintf("%s/%s", c.Params.ByName("Project"), c.Params.ByName("Run"))

	// Load the Run, checking its existence and ACLs.
	r, err := run.LoadRun(ctx, common.RunID(rID), acls.NewRunReadChecker())
	if err != nil {
		errPage(c, err)
		return
	}

	// Reverse entries to get the most recent at the top of the list.
	logEntries, err := run.LoadRunLogEntries(ctx, r.ID)
	if err != nil {
		errPage(c, err)
		return
	}
	nEntries := len(logEntries)
	for i := 0; i < nEntries/2; i++ {
		logEntries[i], logEntries[nEntries-1-i] = logEntries[nEntries-1-i], logEntries[i]
	}

	cls, err := run.LoadRunCLs(ctx, common.RunID(r.ID), r.CLs)
	if err != nil {
		errPage(c, err)
		return
	}

	// Sort a stack of CLs by external ID.
	sort.Slice(cls, func(i, j int) bool { return cls[i].ExternalID < cls[j].ExternalID })

	// Compute next and previous runs for all cls in parallel.
	clsAndLinks, err := computeCLsAndLinks(ctx, cls, r.ID)
	if err != nil {
		errPage(c, err)
		return
	}
	clidToURL := makeURLMap(cls)

	templates.MustRender(ctx, c.Writer, "pages/run_details.html", templates.Args{
		"Run":  r,
		"Logs": logEntries,
		"Cls":  clsAndLinks,
		"RelTime": func(ts time.Time) string {
			return humanize.RelTime(ts, startTime(ctx), "ago", "from now")
		},
		"LogMessage": func(rle *run.LogEntry) string {
			switch v := rle.GetKind().(type) {
			case *run.LogEntry_Info_:
				return v.Info.Message
			case *run.LogEntry_ClSubmitted:
				return StringifySubmissionSuccesses(clidToURL, v.ClSubmitted, int64(len(r.CLs)))
			case *run.LogEntry_SubmissionFailure_:
				return StringifySubmissionFailureReason(clidToURL, v.SubmissionFailure.Event)
			case *run.LogEntry_RunEnded_:
				// This assumes the status of a Run won't change after transitioning to
				// one of the terminal statuses. If the assumption is no longer valid.
				// End status and cancellation reason need to be stored explicitly in
				// the log entry.
				if r.Status == run.Status_CANCELLED {
					switch len(r.CancellationReasons) {
					case 0:
					case 1:
						return fmt.Sprintf("Run is cancelled. Reason: %s", r.CancellationReasons[0])
					default:
						var sb strings.Builder
						sb.WriteString("Run is cancelled. Reasons:")
						for _, reason := range r.CancellationReasons {
							sb.WriteRune('\n')
							sb.WriteString("  * ")
							sb.WriteString(strings.TrimSpace(reason))
						}
						return sb.String()
					}
				}
				return ""
			default:
				return ""
			}
		},
		"AllTryjobs": r.Tryjobs.GetTryjobs(),
	})
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
		i, cl := i, cl
		eg.Go(func() error {
			prev, next, err := getNeighborsByCL(ctx, cl, rid)
			if err != nil {
				return errors.Annotate(err, "unable to get previous and next runs for cl %s", cl.ExternalID).Err()
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
		qb := run.CLQueryBuilder{CLID: cl.ID, Limit: 1}.BeforeInProject(rID)
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
		qb := run.CLQueryBuilder{CLID: cl.ID, Limit: limit}.AfterInProject(rID)
		switch keys, err := qb.GetAllRunKeys(ctx); {
		case err != nil:
			return err
		case len(keys) == limit:
			return errors.Reason("too many Runs (>=%d) after %q", limit, rID).Err()
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

// uiTryjob is fed to the template to draw tryjob chips.
type uiTryjob struct {
	Link, Class, Display string
}

func makeUITryjob(t *run.Tryjob) *uiTryjob {
	// TODO(crbug/1233963):  Make sure we are not leaking any sensitive info
	// based on Read Perms. E.g. internal builder name.
	builder := t.Definition.GetBuildbucket().Builder
	return &uiTryjob{
		Link:    tryjob.ExternalID(t.ExternalId).MustURL(),
		Class:   toCSSClass(t),
		Display: fmt.Sprintf("%s/%s/%s", builder.Project, builder.Bucket, builder.Builder),
	}
}

// toCSSClass returns a css class for styling a tryjob chip based on its
// status and its result's status.
func toCSSClass(t *run.Tryjob) string {
	switch t.GetStatus() {
	case tryjob.Status_PENDING:
		return "not-started"
	case tryjob.Status_CANCELLED:
		return "cancelled"
	case tryjob.Status_TRIGGERED, tryjob.Status_ENDED:
		switch t.GetResult().GetStatus() {
		case tryjob.Result_FAILED_PERMANENTLY, tryjob.Result_FAILED_TRANSIENTLY:
			return "failed"
		case tryjob.Result_SUCCEEDED:
			return "passed"
		default:
			if t.GetStatus() == tryjob.Status_ENDED {
				panic("Tryjob status is ENDED but result status is not set")
			}
			return "running"
		}
	default:
		panic(fmt.Errorf("unknown tryjob status: %s", t.GetStatus()))
	}
}

func logTypeString(rle *run.LogEntry) string {
	switch v := rle.GetKind().(type) {
	case *run.LogEntry_TryjobsUpdated_:
		return "Tryjob Updated"
	case *run.LogEntry_TryjobsRequirementUpdated_:
		return "Tryjob Requirements Updated"
	case *run.LogEntry_ConfigChanged_:
		return "Config Changed"
	case *run.LogEntry_Started_:
		return "Started"
	case *run.LogEntry_Created_:
		return "Created"
	case *run.LogEntry_TreeChecked_:
		if v.TreeChecked.Open {
			return "Tree Found Open"
		}
		return "Tree Found Closed"
	case *run.LogEntry_Info_:
		return v.Info.Label
	case *run.LogEntry_AcquiredSubmitQueue_:
		return "Acquired Submit Queue"
	case *run.LogEntry_ReleasedSubmitQueue_:
		return "Released Submit Queue"
	case *run.LogEntry_Waitlisted_:
		return "Waitlisted for Submit Queue"
	case *run.LogEntry_SubmissionFailure_:
		if v.SubmissionFailure.Event.GetResult() == eventpb.SubmissionResult_FAILED_TRANSIENT {
			return "Transient Submission Failure"
		}
		return "Final Submission Failure"
	case *run.LogEntry_ClSubmitted:
		return "CL Submission"
	case *run.LogEntry_RunEnded_:
		return "CV Finished Work on this Run"
	default:
		return fmt.Sprintf("FIXME: Unknown Kind of LogEntry %T", v)
	}
}

// groupTryjobsByStatus puts tryjobs in the list into separate lists by status.
func groupTryjobsByStatus(tjs []*run.Tryjob) map[string][]*run.Tryjob {
	ret := map[string][]*run.Tryjob{}
	for _, t := range tjs {
		k := strings.Title(strings.ToLower(t.Status.String()))
		ret[k] = append(ret[k], t)
	}
	return ret
}

func indexOf(runs []*adminpb.GetRunResponse, runID common.RunID) int {
	for i := 0; i < len(runs); i++ {
		if runs[i].Id == string(runID) {
			return i
		}
	}
	return -1
}

func makeURLMap(cls []*run.RunCL) map[common.CLID]string {
	ret := make(map[common.CLID]string, len(cls))
	for _, cl := range cls {
		ret[cl.ID] = cl.ExternalID.MustURL()
	}
	return ret
}
