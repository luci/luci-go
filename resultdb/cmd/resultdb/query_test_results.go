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
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/invg"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const maxInvocationGraphSize = 1000

// validateQueryTestResultsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestResultsRequest(req *pb.QueryTestResultsRequest) error {
	if err := pbutil.ValidateTestResultPredicate(req.Predicate, true); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	if req.MaxStaleness != nil {
		if err := pbutil.ValidateMaxStaleness(req.MaxStaleness); err != nil {
			return errors.Annotate(err, "max_staleness").Err()
		}
	}

	return nil
}

// QueryTestResults implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	if err := validateQueryTestResultsRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Prepare a transaction.
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	if in.MaxStaleness != nil {
		st, _ := ptypes.Duration(in.MaxStaleness)
		txn.WithTimestampBound(spanner.MaxStaleness(st))
	}

	// Resolve IDs of the root invocations.
	invIDs, err := resolveRootInvocationIDs(ctx, txn, in.Predicate.Invocation)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "resolved %d root invocation IDs: %q", invIDs)

	// Fetch the invocation graph if needed.
	if incl := in.GetPredicate().GetInvocation().GetInclusions(); incl != pb.InvocationPredicate_NONE {
		stableOnly := incl != pb.InvocationPredicate_ALL
		g, err := invg.FetchGraph(ctx, txn, stableOnly, invIDs...)
		if err != nil {
			return nil, err
		}
		invIDs = invIDs[:0]
		for id := range g.Nodes {
			invIDs = append(invIDs, id)
		}
		logging.Infof(ctx, "resolved %d invocation IDs in the graph: %q", invIDs, invIDs)
	}

	// TODO(nodir): fetch the test results.
	return nil, grpcutil.Unimplemented
}

// resolveRootInvocationIDs returns invocation IDs that satisfy the
// pred.root_predicate.
func resolveRootInvocationIDs(ctx context.Context, txn *spanner.ReadOnlyTransaction, pred *pb.InvocationPredicate) ([]span.InvocationID, error) {
	defer internal.TimeIt(ctx, "resolution of root invocation ids")()
	if pred.GetName() != "" {
		return []span.InvocationID{span.MustParseInvocationName(pred.GetName())}, nil
	}

	var ret []span.InvocationID
	err := txn.Read(ctx, "InvocationsByTag", spanner.Key{span.TagRowID(pred.GetTag())}.AsPrefix(), []string{"InvocationId"}).Do(func(row *spanner.Row) error {
		if len(ret) == maxInvocationGraphSize {
			// TODO(nodir): skip prefix of results based on a cursor.
			return errors.
				Reason("more than %d invocations match tag %q", maxInvocationGraphSize, pbutil.StringPairToString(pred.GetTag())).
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
