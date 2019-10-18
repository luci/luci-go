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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// updateTokenMetadataKey is the metadata.MD key for the secret update token
// required to mutate an invocation.
// It is returned by CreateInvocation RPC in response header metadata,
// and is required by all RPCs mutating an invocation.
const updateTokenMetadataKey = "update-token"

// mutateInvocation checks if the invocation can be mutated and also
// finalizes the invocation if it's deadline is exceeded.
// If the invocation is active, continue with the other mutation(s) in f.
func mutateInvocation(ctx context.Context, invID string, f func(context.Context, *spanner.ReadWriteTransaction) error) error {
	var retErr error

	_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		userToken, err := extractUserUpdateToken(ctx)
		if err != nil {
			return err
		}

		now := clock.Now(ctx)

		var updateToken spanner.NullString
		var state pb.Invocation_State
		var deadline time.Time
		err = span.ReadRow(ctx, txn, "Invocations", spanner.Key{invID}, map[string]interface{}{
			"UpdateToken": &updateToken,
			"State":       &state,
			"Deadline":    &deadline,
		})

		switch {
		case spanner.ErrCode(err) == codes.NotFound:
			return errors.Reason("%q not found", pbutil.InvocationName(invID)).Tag(grpcutil.NotFoundTag).Err()

		case err != nil:
			return err

		case state != pb.Invocation_ACTIVE:
			return errors.Reason("%q is not active", pbutil.InvocationName(invID)).Tag(grpcutil.FailedPreconditionTag).Err()

		case deadline.Before(now):
			retErr = errors.Reason("%q is not active", pbutil.InvocationName(invID)).Tag(grpcutil.FailedPreconditionTag).Err()

			// The invocation has exceeded deadline, finalize it now.
			return txn.BufferWrite([]*spanner.Mutation{
				spanner.UpdateMap("Invocations", map[string]interface{}{
					"InvocationId": invID,
					"State":        int64(pb.Invocation_INTERRUPTED),
					"FinalizeTime": deadline,
				}),
			})

		case !updateToken.Valid:
			return errors.Reason("no update token in active invocation").Tag(grpcutil.InternalTag).Err()

		case userToken != updateToken.StringVal:
			return errors.Reason("invalid update token").Tag(grpcutil.PermissionDeniedTag).Err()

		default:
			return f(ctx, txn)
		}
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
