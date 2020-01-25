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
	"crypto/subtle"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/cmd/recorder/chromium"
	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	day = 24 * time.Hour

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y

	// By default, interrupt the invocation 1h after creation if it is still
	// incomplete.
	defaultInvocationDeadlineDuration = time.Hour

	// To make sure an invocation_task can be performed eventually.
	eventualInvocationTaskProcessAfter = 2 * day
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

		var updateToken spanner.NullString
		var state pb.Invocation_State
		err = span.ReadInvocation(ctx, txn, id, map[string]interface{}{
			"UpdateToken": &updateToken,
			"State":       &state,
		})
		switch {
		case err != nil:
			return err

		case state != pb.Invocation_ACTIVE:
			return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", id.Name())
		}

		if err := validateUserUpdateToken(updateToken, userToken); err != nil {
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
		return "", appstatus.Errorf(codes.Unauthenticated, "missing %s metadata value in the request", updateTokenMetadataKey)

	case len(userToken) > 1:
		return "", appstatus.Errorf(codes.InvalidArgument, "expected exactly one %s metadata value, got %d", updateTokenMetadataKey, len(userToken))

	default:
		return userToken[0], nil
	}
}

func validateUserUpdateToken(updateToken spanner.NullString, userToken string) error {
	if !updateToken.Valid {
		return errors.Reason("no update token in active invocation").Err()
	}

	if subtle.ConstantTimeCompare([]byte(userToken), []byte(updateToken.StringVal)) == 0 {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	return nil
}

func rowOfInvocation(ctx context.Context, inv *pb.Invocation, updateToken, createRequestID string, expectedResultsExpiration time.Duration) map[string]interface{} {
	createTime := pbutil.MustTimestamp(inv.CreateTime)

	row := map[string]interface{}{
		"InvocationId": span.MustParseInvocationName(inv.Name),
		"ShardId":      mathrand.Intn(ctx, span.InvocationShards),
		"State":        inv.State,
		"Interrupted":  inv.Interrupted,
		"Realm":        chromium.Realm, // TODO(crbug.com/1013316): accept realm in the proto

		"InvocationExpirationTime":          createTime.Add(invocationExpirationDuration),
		"ExpectedTestResultsExpirationTime": createTime.Add(expectedResultsExpiration),

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
