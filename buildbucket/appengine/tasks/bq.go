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

package tasks

import (
	"context"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/buildbucket/appengine/model"
)

// ExportBuild saves the build into BiqQuery.
// The returned error has transient.Tag or tq.Fatal in order to tell tq to drop
// or retry.
func ExportBuild(ctx context.Context, buildID int64) error {
	b := &model.Build{ID: buildID}
	switch err := datastore.Get(ctx, b); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "build %d not found when exporting into BQ", buildID).Tag(tq.Fatal).Err()
	case err != nil:
		return errors.Annotate(err, "error fetching builds").Tag(transient.Tag).Err()
	}
	p, err := b.ToProto(ctx, model.NoopBuildMask, nil)
	if err != nil {
		return errors.Annotate(err, "failed to convert build to proto").Err()
	}

	// Clear fields that we don't want in BigQuery.
	p.Infra.Buildbucket.Hostname = ""
	for _, step := range p.GetSteps() {
		step.SummaryMarkdown = ""
		step.MergeBuild = nil
		for _, log := range step.Logs {
			name := log.Name
			log.Reset()
			log.Name = name
		}
	}

	// Set timeout to avoid a hanging call.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	row := &lucibq.Row{
		InsertID: strconv.FormatInt(p.Id, 10),
		Message:  p,
	}
	if err := clients.GetBqClient(ctx).Insert(ctx, "raw", "completed_builds", row); err != nil {
		if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
			return errors.Annotate(err, "bad row for build %d", buildID).Tag(tq.Fatal).Err()
		}
		return errors.Annotate(err, "transient error when inserting BQ for build %d", buildID).Tag(transient.Tag).Err()
	}
	return nil
}
