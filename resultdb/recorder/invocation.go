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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const updateTokenMetadataKey = "update_token"

// mayMutateInvocation returns an error if the requester may not mutate the
// invocation.
// ctx must contain "update_token" metadata value.
func mayMutateInvocation(ctx context.Context, txn span.Txn, invID string) error {
	userToken, err := extractUserUpdateToken(ctx)
	if err != nil {
		return err
	}

	var updateToken spanner.NullString
	var state int64
	err = span.ReadRow(ctx, txn, "Invocations", spanner.Key{invID}, map[string]interface{}{
		"UpdateToken": &updateToken,
		"State":       &state,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return errors.Reason("%q not found", pbutil.InvocationName(invID)).Tag(grpcutil.NotFoundTag).Err()

	case err != nil:
		return err

	case pb.Invocation_State(state) != pb.Invocation_ACTIVE:
		return errors.Reason("%q is not active", pbutil.InvocationName(invID)).Tag(grpcutil.FailedPreconditionTag).Err()

	case !updateToken.Valid:
		return errors.Reason("no update token in active invocation").Tag(grpcutil.InternalTag).Err()

	case userToken != updateToken.StringVal:
		return errors.Reason("invalid update token").Tag(grpcutil.PermissionDeniedTag).Err()

	default:
		return nil
	}
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
