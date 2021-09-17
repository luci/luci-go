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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func runDetails(c *router.Context) {
	rID := fmt.Sprintf("%s/%s", c.Params.ByName("Project"), c.Params.ByName("Run"))

	// TODO(crbug/1233963): check if user has access to this specific Run.

	adminServer := &admin.AdminServer{}
	r, err := adminServer.GetRun(c.Context, &adminpb.GetRunRequest{Run: rID})
	if err != nil {
		errPage(c, err)
		return
	}

	// Reverse entries to get the most recent at the top of the list.
	logEntries := r.LogEntries
	nEntries := len(logEntries)
	for i := 0; i < nEntries/2; i++ {
		logEntries[i], logEntries[nEntries-1-i] = logEntries[nEntries-1-i], logEntries[i]
	}

	cls, err := run.LoadRunCLs(c.Context, common.RunID(r.Id), common.MakeCLIDs(r.Cls...))
	if err != nil {
		errPage(c, err)
		return
	}

	// Sort a stack of CLs by external ID.
	sort.Slice(cls, func(i, j int) bool { return cls[i].ExternalID < cls[j].ExternalID })

	// Compute next and previous runs for all cls in parallel.
	clsAndLinks, err := computeCLsAndLinks(c.Context, adminServer, cls, common.RunID(r.Id))
	if err != nil {
		errPage(c, err)
		return
	}
	clidToURL := makeURLMap(cls)

	templates.MustRender(c.Context, c.Writer, "pages/run_details.html", templates.Args{
		"Run":  r,
		"Logs": logEntries,
		"Cls":  clsAndLinks,
		"RelTime": func(ts time.Time) string {
			return humanize.RelTime(ts, startTime(c.Context), "ago", "from now")
		},
		"LogMessage": func(rle *run.LogEntry) string {
			switch v := rle.GetKind().(type) {
			case *run.LogEntry_Info_:
				return v.Info.Message
			case *run.LogEntry_ClSubmitted:
				return StringifySubmissionSuccesses(clidToURL, v.ClSubmitted, int64(len(r.Cls)))
			case *run.LogEntry_SubmissionFailure_:
				return StringifySubmissionFailureReason(clidToURL, v.SubmissionFailure.Event)
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

func computeCLsAndLinks(ctx context.Context, srv adminpb.AdminServer, cls []*run.RunCL, rid common.RunID) ([]*clAndNeighborRuns, error) {
	ret := make([]*clAndNeighborRuns, len(cls))
	eg, ctx := errgroup.WithContext(ctx)
	for i, cl := range cls {
		i, cl := i, cl
		eg.Go(func() error {
			newer, older, err := getNeighborsByCL(ctx, srv, cls[i], rid)
			if err != nil {
				return errors.Annotate(err, "unable to get previous and next runs for cl %s", cl.ExternalID).Err()
			}
			ret[i] = &clAndNeighborRuns{
				Prev: older,
				Next: newer,
				CL:   cl,
			}
			return nil
		})
	}
	return ret, eg.Wait()
}

func getNeighborsByCL(ctx context.Context, srv adminpb.AdminServer, cl *run.RunCL, rID common.RunID) (common.RunID, common.RunID, error) {
	var newer, older, last common.RunID
	req := &adminpb.SearchRunsRequest{
		Project: rID.LUCIProject(),
		Cl:      &adminpb.GetCLRequest{Id: int64(cl.ID)},
	}
	for {
		// Note that SearchRuns are returned in reverse chronological order
		// i.e. Most recent first.
		resp, err := srv.SearchRuns(ctx, req)
		if err != nil {
			return "", "", err
		}

		i := indexOf(resp.Runs, rID)
		switch {
		case i == -1:
			if resp.GetNextPageToken() == "" {
				return newer, older, nil
			}
			// There are more pages and we haven't found the run of interest.
			// Save the last run of this page in case the first run in the next
			// page is the one of interest.
			last = common.RunID(resp.Runs[len(resp.Runs)-1].Id)
			req.PageToken = resp.GetNextPageToken()
			continue
		case i > 0:
			newer = common.RunID(resp.Runs[i-1].Id)
		case last != common.RunID(""):
			// This is not the first page, but the run of interest
			// is the first in the page. Use the last run of the
			// previous page.
			newer = last
		}

		switch {
		case i < len(resp.Runs)-1:
			older = common.RunID(resp.Runs[i+1].Id)
		case resp.GetNextPageToken() != "":
			// The last run in the page is the one of interest,
			// then the first run in the next page will be the next.
			req.PageToken = resp.GetNextPageToken()
			resp, err = srv.SearchRuns(ctx, req)
			if err != nil {
				return "", "", err
			}
			if len(resp.Runs) > 0 {
				older = common.RunID(resp.Runs[0].Id)
			}
		}
		return newer, older, nil
	}
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
