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

package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/cmd/recorder/chromium"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"
)

const (
	day = 24 * time.Hour

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y

	// Delete expected test results afte this duration since invocation creation.
	expectedTestResultsExpirationDuration = 60 * day // 2mo

	// By default, interrupt the invocation 1h after creation if it is still
	// incomplete.
	defaultInvocationDeadlineDuration = time.Hour
)

// updateTokenMetadataKey is the metadata.MD key for the secret update token
// required to mutate an invocation.
// It is returned by CreateInvocation RPC in response header metadata,
// and is required by all RPCs mutating an invocation.
const updateTokenMetadataKey = "update-token"

// mutateInvocation checks if the invocation can be mutated and also
// finalizes the invocation if it's deadline is exceeded.
// If the invocation is active, continue with the other mutation(s) in f.
func mutateInvocation(ctx context.Context, id span.InvocationID, f func(context.Context, *spanner.ReadWriteTransaction) error) error {
	var retErr error

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		userToken, err := extractUserUpdateToken(ctx)
		if err != nil {
			return err
		}

		now := clock.Now(ctx)

		var updateToken spanner.NullString
		var state pb.Invocation_State
		var deadline time.Time
		err = span.ReadInvocation(ctx, txn, id, map[string]interface{}{
			"UpdateToken": &updateToken,
			"State":       &state,
			"Deadline":    &deadline,
		})

		switch {
		case err != nil:
			return err

		case state != pb.Invocation_ACTIVE:
			return errors.Reason("%q is not active", id.Name()).Tag(grpcutil.FailedPreconditionTag).Err()

		case deadline.Before(now):
			retErr = errors.Reason("%q is not active", id.Name()).Tag(grpcutil.FailedPreconditionTag).Err()

			// The invocation has exceeded deadline, finalize it now.
			return finalizeInvocation(txn, id, true, pbutil.MustTimestampProto(deadline))
		}

		if err = validateUserUpdateToken(updateToken, userToken); err != nil {
			return err
		}

		return f(ctx, txn)
	})

	if err != nil {
		retErr = err
	}
	return retErr
}

func extractUserUpdateToken(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	userToken := md.Get(updateTokenMetadataKey)
	switch {
	case len(userToken) == 0:
		return "", errors.
			Reason("missing %q metadata value in the request", updateTokenMetadataKey).
			Tag(grpcutil.UnauthenticatedTag).
			Err()

	case len(userToken) > 1:
		return "", errors.
			Reason("expected exactly one %q metadata value, got %d", updateTokenMetadataKey, len(userToken)).
			Tag(grpcutil.InvalidArgumentTag).
			Err()

	default:
		return userToken[0], nil
	}
}

func finalizeInvocation(txn *spanner.ReadWriteTransaction, id span.InvocationID, interrupted bool, finalizeTime *tspb.Timestamp) error {
	state := pb.Invocation_COMPLETED
	if interrupted {
		state = pb.Invocation_INTERRUPTED
	}

	return txn.BufferWrite([]*spanner.Mutation{
		span.UpdateMap("Invocations", map[string]interface{}{
			"InvocationId": id,
			"State":        state,
			"FinalizeTime": finalizeTime,
		}),
	})
}

func validateUserUpdateToken(updateToken spanner.NullString, userToken string) error {
	if !updateToken.Valid {
		return errors.Reason("no update token in active invocation").Tag(grpcutil.InternalTag).Err()
	}

	if userToken != updateToken.StringVal {
		return errors.Reason("invalid update token").Tag(grpcutil.PermissionDeniedTag).Err()
	}

	return nil
}

func readInvocationState(ctx context.Context, txn span.Txn, id span.InvocationID) (pb.Invocation_State, error) {
	var state pb.Invocation_State
	err := span.ReadInvocation(ctx, txn, id, map[string]interface{}{"State": &state})
	return state, err
}

func insertInvocationsByTag(invID span.InvocationID, tags []*typepb.StringPair) []*spanner.Mutation {
	muts := make([]*spanner.Mutation, len(tags))
	for i, tag := range tags {
		muts[i] = span.InsertMap("InvocationsByTag", map[string]interface{}{
			"TagId":        span.TagRowID(tag),
			"InvocationId": invID,
		})
	}
	return muts
}

func rowOfInvocation(ctx context.Context, inv *pb.Invocation, updateToken, createRequestID string) map[string]interface{} {
	createTime := pbutil.MustTimestamp(inv.CreateTime)

	row := map[string]interface{}{
		"InvocationId": span.MustParseInvocationName(inv.Name),
		"ShardId":      mathrand.Intn(ctx, span.InvocationShards),
		"State":        inv.State,
		"Realm":        chromium.Realm, // TODO(crbug.com/1013316): accept realm in the proto

		"InvocationExpirationTime":          createTime.Add(invocationExpirationDuration),
		"ExpectedTestResultsExpirationTime": createTime.Add(expectedTestResultsExpirationDuration),

		"UpdateToken": updateToken,

		"CreateTime": inv.CreateTime,
		"Deadline":   inv.Deadline,

		"Tags": inv.Tags,
	}

	if inv.FinalizeTime != nil {
		row["FinalizeTime"] = inv.FinalizeTime
	}

	if createRequestID != "" {
		row["CreateRequestId"] = createRequestID
	}

	return row
}

// insertInvocation returns an spanner mutation that inserts an Invocation row.
// Uses the value of clock.Now(ctx) to compute expiration times.
// Assumes inv is complete and valid; may panic otherwise.
func insertInvocation(ctx context.Context, inv *pb.Invocation, updateToken, createRequestID string) *spanner.Mutation {
	return span.InsertMap("Invocations", rowOfInvocation(ctx, inv, updateToken, createRequestID))
}

// insertOrUpdateInvocation returns an spanner mutation that inserts or updates an Invocation row.
// Uses the value of clock.Now(ctx) to compute expiration times.
// Assumes inv is complete and valid; may panic otherwise.
func insertOrUpdateInvocation(ctx context.Context, inv *pb.Invocation, updateToken, createRequestID string) *spanner.Mutation {
	return span.InsertOrUpdateMap(
		"Invocations", rowOfInvocation(ctx, inv, updateToken, createRequestID))
}
