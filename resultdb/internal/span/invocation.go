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

package span

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ReadInvocation reads one invocation within the transaction.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in util.go.
func ReadInvocation(ctx context.Context, txn Txn, invID string, ptrMap map[string]interface{}) error {
	err := ReadRow(ctx, txn, "Invocations", spanner.Key{invID}, ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return errors.Reason("%q not found", pbutil.InvocationName(invID)).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return errors.Annotate(err, "failed to fetch %q", pbutil.InvocationName((invID))).Err()

	default:
		return nil
	}
}

// ReadInvocationFull reads one invocation struct within the transaction.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadInvocationFull(ctx context.Context, txn Txn, invID string) (*pb.Invocation, error) {
	inv := &pb.Invocation{Name: pbutil.InvocationName(invID)}

	// Populate fields from Invocation table.
	err := ReadInvocation(ctx, txn, invID, map[string]interface{}{
		"State":              &inv.State,
		"CreateTime":         &inv.CreateTime,
		"FinalizeTime":       &inv.FinalizeTime,
		"Deadline":           &inv.Deadline,
		"BaseTestVariantDef": &inv.BaseTestVariantDef,
		"Tags":               &inv.Tags,
	})
	if err != nil {
		return nil, err
	}

	// Populate Inclusions.
	if inv.Inclusions, err = ReadInclusions(ctx, txn, invID); err != nil {
		return nil, err
	}

	return inv, nil
}
