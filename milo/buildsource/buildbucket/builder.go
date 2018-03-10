// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// fetchBuilds fetches builds given a criteria.
// The returned builds are sorted by build creation descending.
// count defines maximum number of builds to fetch; if <0, defaults to 100.
func fetchBuilds(c context.Context, client *bbapi.Service, bucket, builder,
	status string, limit int) ([]*bbapi.ApiCommonBuildMessage, error) {

	search := client.Search()
	search.Bucket(bucket)
	search.Status(status)
	search.Tag(strpair.Format(buildbucket.TagBuilder, builder))
	search.IncludeExperimental(true)

	if limit < 0 {
		limit = 100
	}

	start := clock.Now(c)
	msgs, err := search.Fetch(limit, nil)
	if err != nil {
		return nil, err
	}
	logging.Infof(c, "Fetched %d %s builds in %s", len(msgs), status, clock.Since(c, start))
	return msgs, nil
}

// extractEmail extracts the CL author email from a build message, if available.
// TODO(hinoka, nodir): Delete after buildbucket v2.
func extractEmail(msg *bbapi.ApiCommonBuildMessage) (string, error) {
	var params struct {
		Changes []struct {
			Author struct{ Email string }
		}
	}
	if msg.ParametersJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ParametersJson)).Decode(&params); err != nil {
			return "", errors.Annotate(err, "failed to parse parameters_json").Err()
		}
	}
	if len(params.Changes) == 1 {
		return params.Changes[0].Author.Email, nil
	}
	return "", nil
}

// extractInfo extracts the resulting info text from a build message.
// TODO(hinoka, nodir): Delete after buildbucket v2.
func extractInfo(msg *bbapi.ApiCommonBuildMessage) (string, error) {
	var resultDetails struct {
		// TODO(nodir,iannucci): define a proto for build UI data
		UI struct {
			Info string
		}
	}
	if msg.ResultDetailsJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ResultDetailsJson)).Decode(&resultDetails); err != nil {
			return "", errors.Annotate(err, "failed to parse result_details_json").Err()
		}
	}
	return resultDetails.UI.Info, nil
}

// toMiloBuild converts a buildbucket build to a milo build.
// In case of an error, returns a build with a description of the error
// and logs the error.
func toMiloBuild(c context.Context, msg *bbapi.ApiCommonBuildMessage) (*ui.MiloBuild, error) {
	// Parse the build message into a buildbucket.Build struct, filling in the
	// input and output properties that we expect to receive.
	var b buildbucket.Build
	var props struct {
		Revision string `json:"revision"`
	}
	var resultDetails struct {
		GotRevision string `json:"got_revision"`
	}
	b.Input.Properties = &props
	b.Output.Properties = &resultDetails
	if err := b.ParseMessage(msg); err != nil {
		return nil, err
	}
	// TODO(hinoka,nodir): Replace the following lines after buildbucket v2.
	info, err := extractInfo(msg)
	if err != nil {
		return nil, err
	}
	email, err := extractEmail(msg)
	if err != nil {
		return nil, err
	}

	result := &ui.MiloBuild{
		Summary: ui.BuildComponent{
			PendingTime:   ui.NewInterval(c, b.CreationTime, b.StartTime),
			ExecutionTime: ui.NewInterval(c, b.StartTime, b.CompletionTime),
			Status:        parseStatus(b.Status),
		},
		Trigger: &ui.Trigger{},
	}
	// Add in revision information, if available.
	if resultDetails.GotRevision != "" {
		result.Trigger.Commit.Revision = ui.NewEmptyLink(resultDetails.GotRevision)
	}
	if props.Revision != "" {
		result.Trigger.Commit.RequestRevision = ui.NewEmptyLink(props.Revision)
	}

	if b.Experimental {
		result.Summary.Text = []string{"Experimental"}
	}
	if info != "" {
		result.Summary.Text = append(result.Summary.Text, strings.Split(info, "\n")...)
	}

	for _, bs := range b.BuildSets {
		// ignore rietveld.
		cl, ok := bs.(*buildbucket.GerritChange)
		if !ok {
			continue
		}

		// support only one CL per build.
		result.Blame = []*ui.Commit{{
			Changelist:      ui.NewPatchLink(cl),
			RequestRevision: ui.NewLink(props.Revision, "", fmt.Sprintf("request revision %s", props.Revision)),
		}}

		result.Blame[0].AuthorEmail = email
		break
	}

	if b.Number != nil {
		numStr := strconv.Itoa(*b.Number)
		result.Summary.Label = ui.NewLink(
			numStr,
			fmt.Sprintf("/p/%s/builders/%s/%s/%s", b.Project, b.Bucket, b.Builder, numStr),
			fmt.Sprintf("build #%s", numStr))
	} else {
		idStr := strconv.FormatInt(b.ID, 10)
		result.Summary.Label = ui.NewLink(
			idStr,
			fmt.Sprintf("/p/%s/builds/b%s", b.Project, idStr),
			fmt.Sprintf("build #%s", idStr))
	}
	return result, nil
}

func getDebugBuilds(c context.Context, bucket, builder string, maxCompletedBuilds int, target *ui.Builder) error {
	// ../buildbucket below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	resFile, err := os.Open(filepath.Join(
		"..", "buildbucket", "testdata", bucket, builder+".json"))
	if err != nil {
		return err
	}
	defer resFile.Close()

	res := &bbapi.ApiSearchResponseMessage{}
	if err := json.NewDecoder(resFile).Decode(res); err != nil {
		return err
	}

	for _, bb := range res.Builds {
		mb, err := toMiloBuild(c, bb)
		if err != nil {
			return err
		}
		switch mb.Summary.Status {
		case model.NotRun:
			target.PendingBuilds = append(target.PendingBuilds, mb)

		case model.Running:
			target.CurrentBuilds = append(target.CurrentBuilds, mb)

		case model.Success, model.Failure, model.InfraFailure, model.Warning:
			if len(target.FinishedBuilds) < maxCompletedBuilds {
				target.FinishedBuilds = append(target.FinishedBuilds, mb)
			}

		default:
			panic("impossible")
		}
	}
	return nil
}

func getHost(c context.Context) (string, error) {
	settings := common.GetSettings(c)
	if settings.Buildbucket == nil || settings.Buildbucket.Host == "" {
		return "", errors.New("missing buildbucket host in settings")
	}
	return settings.Buildbucket.Host, nil
}

// GetBuilder is used by buildsource.BuilderID.Get to obtain the resp.Builder.
func GetBuilder(c context.Context, bucket, builder string, limit int) (*ui.Builder, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	if limit < 0 {
		limit = 20
	}

	result := &ui.Builder{
		Name: builder,
	}
	if host == "debug" {
		return result, getDebugBuilds(c, bucket, builder, limit, result)
	}
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}

	fetch := func(statusFilter string, limit int) error {
		msgs, err := fetchBuilds(c, client, bucket, builder, statusFilter, limit)
		if err != nil {
			logging.Errorf(c, "Could not fetch %s builds: %s", statusFilter, err)
			return err
		}
		for _, m := range msgs {
			mb, err := toMiloBuild(c, m)
			if err != nil {
				return errors.Annotate(err, "failed to convert build %d to milo build", m.Id).Err()
			}
			switch mb.Summary.Status {
			case model.NotRun:
				result.PendingBuilds = append(result.PendingBuilds, mb)
			case model.Running:
				result.CurrentBuilds = append(result.CurrentBuilds, mb)
			default:
				result.FinishedBuilds = append(result.FinishedBuilds, mb)
			}
		}
		return nil
	}
	return result, parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return fetch(bbapi.StatusScheduled, -1)
		}
		work <- func() error {
			return fetch(bbapi.StatusStarted, -1)
		}
		work <- func() error {
			return fetch(bbapi.StatusCompleted, limit)
		}
	})
}
