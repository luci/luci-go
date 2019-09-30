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
	"google.golang.org/grpc/codes"
	"go.chromium.org/luci/appengine/meta"
	"google.golang.org/grpc/metadata"
	"go.chromium.org/luci/grpc/grpcutil"
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/storage"
)

const updateTokenMetadataKey = "update_token"

// mayMutateInvocation returns an error if the requester may not mutate the
// invocation.
// ctx must contain "update_token" metadata value.
func mayMutateInvocation(ctx context.Context, txn *spanner.ReadWriteTransaction, invID, updateToken string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	actualTokens := md.Get(updateTokenMetadataKey)
	switch {
	case len(actualTokens) == 0:
		return errors.Reason("missing %q metadata value in the request", updateTokenMetadataKey).Tag(grpcutil.UnauthenticatedTag).Err()
	case len(actualTokens) > 1:
		return errors.Reason("expected exactly one %q metadata value, got %d", updateTokenMetadataKey, len(actualTokens)).Tag(grpcutil.InvalidArgumentTag).Err()
	}

	row, err := tx.ReadRow(ctx, "Invocations", spanner.Key{invID}, "UpdateToken", "State")
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return errors.Reason("invocation not found").Tag(grpcutil.NotFoundTag).Err()
	case err != nil:
		return err
	}

	var string actualUpdateToken
	var state pb.Invocation_State
	switch err := row.Columns(&updateToken, &state) {
	case err != nil:
		return err

	case actualTokens[0] != updateToken:
		return errors.Reason("invalid update token").Tag(grpcutil.PermissionDeniedTag).Err()

	case state != pb.Invocation_ACTIVE {
		return errors.Reason("invocation %q is not active", invName).Tag(grpcutil.FailedPreconditionTag).Err()

	default:
		return nil
	}
}
