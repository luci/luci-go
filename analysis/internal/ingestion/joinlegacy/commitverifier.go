// Copyright 2022 The LUCI Authors.
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

package joinlegacy

import (
	"context"
	"fmt"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	cvv1 "go.chromium.org/luci/cv/api/v1"

	"go.chromium.org/luci/analysis/internal/cv"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/ingestion/controllegacy"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	// TODO(chanli@@) Removing the hosts after CVPubSub and GetRun RPC added them.
	// Host name of buildbucket.
	bbHost = "cr-buildbucket.appspot.com"
)

var (
	runIDRe = regexp.MustCompile(`^projects/(.*)/runs/(.*)$`)

	// Automation service accounts.
	automationAccountRE = regexp.MustCompile(`^.*@.*\.gserviceaccount\.com$`)
)

// JoinCVRun notifies ingestion that the given LUCI CV Run has finished.
// Ingestion tasks are created for buildbucket builds when all required data
// for a build (including any associated LUCI CV run) is available.
func JoinCVRun(ctx context.Context, psRun *cvv1.PubSubRun) (project string, processed bool, err error) {
	project, runID, err := parseRunID(psRun.Id)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "failed to parse run ID").Err()
	}

	// We do not check if the project is configured in LUCI Analysis,
	// as CV runs can include projects from other projects.
	// It is the projects of these builds that the data
	// gets ingested into.

	run, err := getRun(ctx, psRun)
	code := status.Code(err)
	if code == codes.NotFound {
		logging.Warningf(ctx, "CV run %s for project %s not found (or LUCI Analysis does not have access to read it).",
			runID, project)
		// Treat as a permanent error.
		return project, false, errors.Annotate(err, "failed to get run").Err()
	}
	if err != nil {
		// Treat as transient error.
		return project, false, transient.Tag.Apply(errors.Annotate(err, "failed to get run").Err())
	}
	if run.GetCreateTime() == nil {
		return project, false, errors.New("could not get create time for the run")
	}

	owner := "user"
	if automationAccountRE.MatchString(run.Owner) {
		owner = "automation"
	}

	mode, err := pbutil.PresubmitRunModeFromString(run.Mode)
	if err != nil {
		return project, false, errors.Annotate(err, "failed to parse run mode").Err()
	}

	presubmitResultByBuildID := make(map[string]*ctlpb.PresubmitResult)
	for _, tji := range run.TryjobInvocations {
		for attemptIndex, attempt := range tji.Attempts {
			b := attempt.GetResult().GetBuildbucket()
			if b == nil {
				// Non build-bucket result.
				continue
			}
			if attempt.Reuse {
				// Do not ingest re-used tryjobs.
				// Builds should be ingested with the CV run that initiated
				// them, not a CV run that re-used them.
				// Tryjobs can also be marked re-used if they were user
				// initiated through gerrit. In this case, the build would
				// not have been tagged as being part of a CV run (e.g.
				// through user_agent: cq), so it will not expect to be
				// joined to a CV run.
				continue
			}

			buildID := controllegacy.BuildID(bbHost, b.Id)
			if _, ok := presubmitResultByBuildID[buildID]; ok {
				logging.Warningf(ctx, "CV Run %s has build %s as tryjob multiple times, ignoring the second occurances", psRun.Id, buildID)
				continue
			}

			status, err := pbutil.PresubmitRunStatusFromLUCICV(run.Status)
			if err != nil {
				return project, false, errors.Annotate(err, "failed to parse run status").Err()
			}

			presubmitResultByBuildID[buildID] = &ctlpb.PresubmitResult{
				PresubmitRunId: &pb.PresubmitRunId{
					System: "luci-cv",
					Id:     fmt.Sprintf("%s/%s", project, runID),
				},
				Status:       status,
				Mode:         mode,
				Owner:        owner,
				CreationTime: run.CreateTime,
				Critical:     tji.Critical && attemptIndex == 0,
			}
		}
	}

	if err := JoinPresubmitResult(ctx, presubmitResultByBuildID, project); err != nil {
		return project, true, errors.Annotate(err, "joining presubmit results").Err()
	}

	return project, true, nil
}

func parseRunID(runID string) (project string, run string, err error) {
	m := runIDRe.FindStringSubmatch(runID)
	if m == nil {
		return "", "", errors.Reason("run ID does not match %s", runIDRe).Err()
	}
	return m[1], m[2], nil
}

// getRun gets the full Run message by make a GetRun RPC to CV.
//
// Currently we're calling cv.v0.Runs.GetRun, and should switch to v1 when it's
// ready to use.
func getRun(ctx context.Context, psRun *cvv1.PubSubRun) (*cvv0.Run, error) {
	c, err := cv.NewClient(ctx, psRun.Hostname)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create cv client").Err()
	}
	req := &cvv0.GetRunRequest{
		Id: psRun.Id,
	}
	return c.GetRun(ctx, req)
}
