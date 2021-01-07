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

package migration

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

type finishedRun struct {
	_kind string `gae:"$kind,migration.FinishedRun"`

	ID      common.RunID `gae:"$id"`
	Status  run.Status   `gae:",noindex"`
	EndTime time.Time    `gae:",noindex"`
}

// FinalizeRun update the Run with the Finished Run reported by CQDaemon.
func FinalizeRun(ctx context.Context, r *run.Run) error {
	fr := finishedRun{ID: r.ID}
	switch err := datastore.Get(ctx, &fr); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Reason("Run %q hasn't been reported finished by CQDaemon", r.ID).Err()
	case err != nil:
		return errors.Annotate(err, "failed to fetch finishedRun %q reported by CQDaemon", r.ID).Tag(transient.Tag).Err()
	}
	r.Status = fr.Status
	r.EndTime = fr.EndTime
	return nil
}

var endedStatusMapping = map[bigquery.AttemptStatus]run.Status{
	bigquery.AttemptStatus_SUCCESS:       run.Status_SUCCEEDED,
	bigquery.AttemptStatus_FAILURE:       run.Status_FAILED,
	bigquery.AttemptStatus_INFRA_FAILURE: run.Status_FAILED,
	bigquery.AttemptStatus_ABORTED:       run.Status_CANCELLED,
}

func storeFinishedRun(ctx context.Context, r *migrationpb.Run) error {
	fr := finishedRun{
		ID:      common.RunID(r.GetId()),
		EndTime: r.GetAttempt().GetEndTime().AsTime(),
	}
	fr.Status = endedStatusMapping[r.GetAttempt().GetStatus()]
	if fr.Status == run.Status_STATUS_UNSPECIFIED {
		return errors.Reason("expected ended status for [attempt=%q,run=%q]; got %s", r.GetAttempt().GetKey(), r.GetId(), r.GetAttempt().GetStatus()).Err()
	}
	return errors.Annotate(datastore.Put(ctx, &fr), "failed to put finishedRun %q", r.GetId()).Tag(transient.Tag).Err()
}
