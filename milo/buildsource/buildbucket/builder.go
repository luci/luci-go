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
	"time"

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

// toMiloBuild converts a buildbucket build to a milo build.
// In case of an error, returns a build with a description of the error
// and logs the error.
func toMiloBuild(c context.Context, msg *bbapi.ApiCommonBuildMessage) (*ui.BuildSummary, error) {
	var b buildbucket.Build
	if err := b.ParseMessage(msg); err != nil {
		return nil, err
	}

	var params struct {
		Changes []struct {
			Author struct{ Email string }
		}
		Properties struct {
			Revision string `json:"revision"`
		}
	}
	if msg.ParametersJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ParametersJson)).Decode(&params); err != nil {
			return nil, errors.Annotate(err, "failed to parse parameters_json of build %d", b.ID).Err()
		}
	}

	var resultDetails struct {
		Properties struct {
			GotRevision string `json:"got_revision"`
		}
		// TODO(nodir,iannucci): define a proto for build UI data
		UI struct {
			Info string
		}
	}
	if msg.ResultDetailsJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ResultDetailsJson)).Decode(&resultDetails); err != nil {
			return nil, errors.Annotate(err, "failed to parse result_details_json of build %d", b.ID).Err()
		}
	}

	duration := func(start, end time.Time) time.Duration {
		if start.IsZero() {
			return 0
		}
		if end.IsZero() {
			end = clock.Now(c)
		}
		if end.Before(start) {
			return 0
		}
		return end.Sub(start)
	}

	result := &ui.BuildSummary{
		Revision: resultDetails.Properties.GotRevision,
		Status:   parseStatus(b.Status),
		PendingTime: ui.Interval{
			Started:  b.CreationTime,
			Finished: b.StartTime,
			Duration: duration(b.CreationTime, b.StartTime),
		},
		ExecutionTime: ui.Interval{
			Started:  b.StartTime,
			Finished: b.CompletionTime,
			Duration: duration(b.StartTime, b.CompletionTime),
		},
	}
	if result.Revision == "" {
		result.Revision = params.Properties.Revision
	}
	if b.Experimental {
		result.Text = []string{"Experimental"}
	}
	if resultDetails.UI.Info != "" {
		result.Text = append(result.Text, strings.Split(resultDetails.UI.Info, "\n")...)
	}

	for _, bs := range b.BuildSets {
		// ignore rietveld.
		cl, ok := bs.(*buildbucket.GerritChange)
		if !ok {
			continue
		}

		// support only one CL per build.
		result.Blame = []*ui.Commit{{
			Changelist: ui.NewLink(fmt.Sprintf("Gerrit CL %d", cl.Change), cl.URL(),
				fmt.Sprintf("gerrit changelist %d", cl.Change)),
			RequestRevision: ui.NewLink(params.Properties.Revision, "", fmt.Sprintf("request revision %s", params.Properties.Revision)),
		}}

		if len(params.Changes) == 1 {
			result.Blame[0].AuthorEmail = params.Changes[0].Author.Email
		}
		break
	}

	if b.Number != nil {
		numStr := strconv.Itoa(*b.Number)
		result.Link = ui.NewLink(
			numStr,
			fmt.Sprintf("/p/%s/builders/%s/%s/%s", b.Project, b.Bucket, b.Builder, numStr),
			fmt.Sprintf("build #%s", numStr))
	} else {
		idStr := strconv.FormatInt(b.ID, 10)
		result.Link = ui.NewLink(
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
		switch mb.Status {
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
			b, err := toMiloBuild(c, m)
			if err != nil {
				return errors.Annotate(err, "failed to convert build %d to milo build", m.Id).Err()
			}
			switch b.Status {
			case model.NotRun:
				result.PendingBuilds = append(result.PendingBuilds, b)
			case model.Running:
				result.CurrentBuilds = append(result.CurrentBuilds, b)
			default:
				result.FinishedBuilds = append(result.FinishedBuilds, b)
			}
		}
		return nil
	}
	return result, parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return fetch(bbapi.StatusFilterIncomplete, -1)
		}
		work <- func() error {
			return fetch(bbapi.StatusCompleted, limit)
		}
	})
}
