// Copyright 2019 The LUCI Authors.
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

package recorder

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	day = 24 * time.Hour

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y

	// By default, finalize the invocation 1h after creation if it is still
	// incomplete.
	defaultInvocationDeadlineDuration = time.Hour

	// To make sure an invocation_task can be performed eventually.
	eventualInvocationTaskProcessAfter = 2 * day
)

// UpdateTokenMetadataKey is the metadata.MD key for the secret update token
// required to mutate an invocation.
// It is returned by CreateInvocation RPC in response header metadata,
// and is required by all RPCs mutating an invocation.
const UpdateTokenMetadataKey = "update-token"

// invocationTokenKind generates and validates tokens issued to authorize
// updating a given invocation.
var invocationTokenKind = tokens.TokenKind{
	Algo:      tokens.TokenAlgoHmacSHA256,
	SecretKey: "invocation_tokens_secret",
	Version:   1,
}

// generateInvocationToken generates an update token for a given invocation.
func generateInvocationToken(ctx context.Context, invID span.InvocationID) (string, error) {
	// The token should last as long as a build is allowed to run.
	// Buildbucket has a max of 2 days, so one week should be enough even
	// for other use cases.
	return invocationTokenKind.Generate(ctx, []byte(invID), nil, 7*day) // One week.
}

// validateInvocationToken validates an update token for a given invocation,
// returning an error if the token is invalid, nil otherwise.
func validateInvocationToken(ctx context.Context, token string, invID span.InvocationID) error {
	_, err := invocationTokenKind.Validate(ctx, token, []byte(invID))
	return err
}

// mutateInvocation checks if the invocation can be mutated and also
// finalizes the invocation if it's deadline is exceeded.
// If the invocation is active, continue with the other mutation(s) in f.
func mutateInvocation(ctx context.Context, id span.InvocationID, f func(context.Context, *spanner.ReadWriteTransaction) error) error {
	var retErr error

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateInvocationToken(ctx, token, id); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {

		state, err := span.ReadInvocationState(ctx, txn, id)
		switch {
		case err != nil:
			return err

		case state != pb.Invocation_ACTIVE:
			return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", id.Name())
		}

		return f(ctx, txn)
	})

	if err != nil {
		retErr = err
	}
	return retErr
}

func extractUpdateToken(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	token := md.Get(UpdateTokenMetadataKey)
	switch {
	case len(token) == 0:
		return "", appstatus.Errorf(codes.Unauthenticated, "missing %s metadata value in the request", UpdateTokenMetadataKey)

	case len(token) > 1:
		return "", appstatus.Errorf(codes.InvalidArgument, "expected exactly one %s metadata value, got %d", UpdateTokenMetadataKey, len(token))

	default:
		return token[0], nil
	}
}

// rowOfInvocation returns Invocation row values to be inserted to create the
// invocation.
// inv.CreateTime is ignored in favor of spanner.CommitTime.
func (s *recorderServer) rowOfInvocation(ctx context.Context, inv *pb.Invocation, createRequestID string) map[string]interface{} {
	now := clock.Now(ctx).UTC()
	row := map[string]interface{}{
		"InvocationId": span.MustParseInvocationName(inv.Name),
		"ShardId":      mathrand.Intn(ctx, span.InvocationShards),
		"State":        inv.State,
		"Realm":        realms.Join("chromium", "public"), // TODO(crbug.com/1013316): accept realm in the proto
		"CreatedBy":    inv.CreatedBy,

		"InvocationExpirationTime":          now.Add(invocationExpirationDuration),
		"ExpectedTestResultsExpirationTime": now.Add(s.ExpectedResultsExpiration),

		"CreateTime": spanner.CommitTimestamp,
		"Deadline":   inv.Deadline,

		"Tags":             inv.Tags,
		"ProducerResource": inv.ProducerResource,
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
