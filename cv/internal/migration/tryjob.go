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
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
)

const reportedTryjobsKind = "migration.ReportedTryjobs"

// ReportTryjobs stores snapshots of Run's tryjobs reported by CQDaemon.
type ReportedTryjobs struct {
	_kind string `gae:"$kind,migration.ReportedTryjobs"`
	// ID records RunID & when entity was inserted, see makeReportedTryjobsID().
	ID string `gae:"$id"`
	// Payload is what CQDaemon has reported.
	Payload *migrationpb.ReportTryjobsRequest
}

// cqdDeletedAfter is very pessimistic deadline for deleting CQDaemon.
//
// Changing this value will corrupt the data.
var cqdDeletedAfter = time.Date(2025, 12, 31, 11, 59, 59, 0, time.UTC)

// makeReportedTryjobsID encodes Run and ReportTime in a string such that
// lexicogrpahic order sorts by (Run ASC, ReportTime DESC) order.
func makeReportedTryjobsID(rid common.RunID, reportTime time.Time) string {
	dur := cqdDeletedAfter.Sub(reportTime)
	if dur < 0 {
		panic(fmt.Errorf("can't report tryjobs after pessimistic deadline to delete CQDaemon %s", cqdDeletedAfter))
	}
	return fmt.Sprintf("%s/%020d", string(rid), int64(dur))
}

func (r *ReportedTryjobs) RunID() common.RunID {
	i := strings.LastIndex(r.ID, "/")
	if i <= 0 {
		panic(fmt.Errorf("invalid ReportedTryjobs ID %q", r.ID))
	}
	return common.RunID(r.ID[:i])
}

func (r *ReportedTryjobs) ReportTime() time.Time {
	i := strings.LastIndex(r.ID, "/")
	if i <= 0 {
		panic(fmt.Errorf("invalid ReportedTryjobs ID %q", r.ID))
	}
	v, err := strconv.ParseInt(r.ID[i+1:], 10, 64)
	if err != nil {
		panic(fmt.Errorf("invalid ReportedTryjobs ID %q: %s", r.ID, err))
	}
	return cqdDeletedAfter.Add(-time.Duration(v))
}

func saveReportedTryjobs(ctx context.Context, req *migrationpb.ReportTryjobsRequest, notify func(ctx context.Context, id string) error) error {
	id := makeReportedTryjobsID(common.RunID(req.GetRunId()), clock.Now(ctx))
	try := 0
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		try++
		v := ReportedTryjobs{ID: id, Payload: req}
		switch err := datastore.Get(ctx, &v); {
		case err == datastore.ErrNoSuchEntity:
			// expected.
		case err != nil:
			return err
		default:
			logging.Warningf(ctx, "%q in %d-th try: already exists", id, try)
			return nil
		}
		if err := datastore.Put(ctx, &v); err != nil {
			return err
		}
		return notify(ctx, id)
	}, nil)
	return errors.Annotate(err, "failed to record ReportedTryjobs %q after %d tries", id, try).Tag(transient.Tag).Err()
}

// ListReportedTryjobs returns all ReportedTryjobs records reported by CQDaemon
// since a given time, ordered by time DESC.
//
// `after` is exclusive.
func ListReportedTryjobs(ctx context.Context, rid common.RunID, after time.Time, limit int32) ([]*ReportedTryjobs, error) {
	minID := fmt.Sprintf("%s/0", rid)
	maxID := fmt.Sprintf("%s/:", rid)
	if !after.IsZero() {
		maxID = makeReportedTryjobsID(rid, after)
	}
	q := datastore.NewQuery(reportedTryjobsKind)
	q = q.Gt("__key__", datastore.MakeKey(ctx, reportedTryjobsKind, minID))
	q = q.Lt("__key__", datastore.MakeKey(ctx, reportedTryjobsKind, maxID))
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []*ReportedTryjobs
	if err := datastore.GetAll(ctx, q, &out); err != nil {
		return nil, errors.Annotate(err, "failed to query ReportedTryjobs").Tag(transient.Tag).Err()
	}
	return out, nil
}
