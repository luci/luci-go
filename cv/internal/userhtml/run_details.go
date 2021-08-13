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
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
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

	templates.MustRender(c.Context, c.Writer, "pages/run_details.html", templates.Args{
		"Run":  r,
		"Logs": logEntries,
		"Cls":  cls,
		"RelTime": func(ts time.Time) string {
			return humanize.RelTime(ts, startTime(c.Context), "ago", "from now")
		},
		"AllTryjobs": r.Tryjobs.GetTryjobs(),
	})
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

func logTypeString(rle *run.LogEntry) (string, error) {
	switch v := rle.GetKind().(type) {
	case *run.LogEntry_TryjobsUpdated_:
		return "Tryjob Updated", nil
	case *run.LogEntry_TryjobsRequirementUpdated_:
		return "Tryjob Requirements Updated", nil
	case *run.LogEntry_ConfigChanged_:
		return "Config Changed", nil
	case *run.LogEntry_Started_:
		return "Started", nil
	case *run.LogEntry_Created_:
		return "Created", nil
	default:
		return "", fmt.Errorf("unknown kind of LogEntry %T", v)
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
