// Copyright 2020 The LUCI Authors.
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

package deriver

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal/invocations"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	day = 24 * time.Hour

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y
)

// rowOfInvocation returns Invocation row values to be inserted to create the
// invocation.
// inv.CreateTime is ignored in favor of spanner.CommitTime.
func (s *deriverServer) rowOfInvocation(ctx context.Context, inv *pb.Invocation, createRequestID string, trCount int64) map[string]interface{} {
	now := clock.Now(ctx).UTC()
	row := map[string]interface{}{
		"InvocationId": invocations.MustParseName(inv.Name),
		"ShardId":      mathrand.Intn(ctx, invocations.Shards),
		"State":        inv.State,
		"Realm":        realms.Join("chromium", "public"),

		"InvocationExpirationTime":          now.Add(invocationExpirationDuration),
		"ExpectedTestResultsExpirationTime": now.Add(s.ExpectedResultsExpiration),

		"CreateTime": spanner.CommitTimestamp,
		"Deadline":   inv.Deadline,

		"Tags":             inv.Tags,
		"ProducerResource": inv.ProducerResource,
		"TestResultCount":  trCount,
	}

	if inv.State == pb.Invocation_FINALIZED {
		// We are ignoring the provided inv.FinalizeTime because it would not
		// make sense to have an invocation finalized before it was created,
		// yet attempting to set this in the future would fail the sql schema
		// restriction for columns that allow commit timestamp.
		// Note this function is only used for setting FinalizeTime by derive
		// invocation, which is planned to be superseded by other mechanisms.
		row["FinalizeTime"] = spanner.CommitTimestamp
	}

	if createRequestID != "" {
		row["CreateRequestId"] = createRequestID
	}

	if len(inv.BigqueryExports) != 0 {
		bqExports := make([][]byte, len(inv.BigqueryExports))
		for i, msg := range inv.BigqueryExports {
			var err error
			if bqExports[i], err = proto.Marshal(msg); err != nil {
				panic(fmt.Sprintf("failed to marshal BigQueryExport to bytes: %s", err))
			}
		}
		row["BigQueryExports"] = bqExports
	}

	return row
}
