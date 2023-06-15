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

package tasks

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// sendOnBuildCompletion sends a bunch of related events when build is reaching
// to an end status, e.g. finalizing the resultdb invocation, exporting to Bq,
// and notify pubsub topics.
func sendOnBuildCompletion(ctx context.Context, bld *model.Build) error {
	bld.ClearLease()

	return parallel.FanOutIn(func(tks chan<- func() error) {
		tks <- func() error {
			return errors.Annotate(NotifyPubSub(ctx, bld), "failed to enqueue pubsub notification task: %d", bld.ID).Err()
		}
		tks <- func() error {
			return errors.Annotate(ExportBigQuery(ctx, bld.ID, strings.Contains(bld.ExperimentsString(), buildbucket.ExperimentBqExporterGo)), "failed to enqueue bigquery export task: %d", bld.ID).Err()
		}
		tks <- func() error {
			return errors.Annotate(FinalizeResultDB(ctx, &taskdefs.FinalizeResultDBGo{BuildId: bld.ID}), "failed to enqueue resultDB finalization task: %d", bld.ID).Err()
		}
	})
}

// SendOnBuildStatusChange sends cloud tasks if a build's top level status changes.
func SendOnBuildStatusChange(ctx context.Context, bld *model.Build) error {
	if datastore.Raw(ctx) == nil || datastore.CurrentTransaction(ctx) == nil {
		return errors.Reason("must enqueue cloud tasks that are triggered by build status update in a transaction").Err()
	}
	switch {
	case bld.Proto.Status == pb.Status_STARTED:
		if err := NotifyPubSub(ctx, bld); err != nil {
			logging.Debugf(ctx, "failed to notify pubsub about starting %d: %s", bld.ID, err)
		}
	case protoutil.IsEnded(bld.Proto.Status):
		return sendOnBuildCompletion(ctx, bld)
	}
	return nil
}
