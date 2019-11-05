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

	"go.chromium.org/luci/resultdb/internal/invg"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateQueryTestResults returns an error if req is determined to be invalid.
func validateQueryTestResults(req *pb.QueryTestResultsRequest) error {
	if err := pbutil.ValidateTestResultQuery(req.Query); err != nil {
		return errors.Annotate(err, "query").Err()
	}

	return nil
}

// QueryTestResults implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	if err := validateQueryTestResults(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()

	if in.Query.MaxStaleness != nil {
		st, _ := ptypes.Duration(in.Query.MaxStaleness)
		txn.WithTimestampBound(spanner.MaxStaleness(st))
	}

	start := clock.Now(ctx)
	invIDs, err := resolveInvocationIDs(ctx, txn, in.Query.Predicate.Invocation)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "resolved invocation IDs to %q in %s", invIDs, clock.Since(ctx, start))

	start = clock.Now(ctx)
	g, err := invg.FetchGraph(ctx, txn, invIDs...)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "fetched %d invocations in %s", len(g.Nodes), clock.Since(ctx, start))

	return nil, grpcutil.Unimplemented
}

// resolveInvocationIDs returns invocation IDs that satisfy the predicate.
func resolveInvocationIDs(ctx context.Context, txn *spanner.ReadOnlyTransaction, pred *pb.InvocationPredicate) ([]span.InvocationID, error) {
	if pred.GetName() != "" {
		return []span.InvocationID{span.MustParseInvocationName(pred.GetName())}, nil
	}

	var ret []span.InvocationID
	err := txn.Read(ctx, "InvocationsByTag", spanner.Key{span.TagRowID(pred.GetTag())}.AsPrefix(), []string{"InvocationId"}).Do(func(row *spanner.Row) error {
		if len(ret) == 1000 {
			// TODO(nodir): skip prefix of results based on a cursor.
			return errors.
				Reason("more than 1000 invocations match tag %q", pbutil.StringPairToString(pred.GetTag())).
				Tag(grpcutil.FailedPreconditionTag).
				Err()
		}
		var id span.InvocationID
		if err := span.FromSpanner(row, &id); err != nil {
			return err
		}
		ret = append(ret, id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
