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

// This package is responsible for preparing runs (as Attempt protos) to
// BigQuery at the end of a run.

package bq

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/migration"
)

// fetchCQDAttempt fetches the Attempt from a Run that was reported by
// CQDaemon, and prepares it to be sent to BigQuery.
//
// This function is used only during migration, and should be removed and replaced
// after migration from CQDaemon is complete.
func fetchCQDAttempt(ctx context.Context, id common.RunID) (*cvbqpb.Attempt, error) {
	v := migration.VerifiedCQDRun{ID: id}
	err := datastore.Get(ctx, &v)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.Annotate(err, "no run found trying to get Attempt").Err()
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to fetch run").Tag(transient.Tag).Err()
	}
	return v.Payload.Run.Attempt, nil
}
