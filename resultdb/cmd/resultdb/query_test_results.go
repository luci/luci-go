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
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const maxInvocationGraphSize = 1000

// queryRequest is implemented by *pb.QueryTestResultsRequest and
// *pb.QueryTestExonerationsRequest.
type queryRequest interface {
	GetInvocations() []string
	GetPageSize() int32
	GetMaxStaleness() *durpb.Duration
}

// validateQueryRequest returns a non-nil error if req is determined to be
// invalid.
func validateQueryRequest(req queryRequest) error {
	if len(req.GetInvocations()) == 0 {
		return errors.Reason("invocations: unspecified").Err()
	}
	for _, name := range req.GetInvocations() {
		if err := pbutil.ValidateInvocationName(name); err != nil {
			return errors.Annotate(err, "invocations: %q", name).Err()
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	if req.GetMaxStaleness() != nil {
		if err := pbutil.ValidateMaxStaleness(req.GetMaxStaleness()); err != nil {
			return errors.Annotate(err, "max_staleness").Err()
		}
	}

	return nil
}

// validateQueryTestResultsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestResultsRequest(req *pb.QueryTestResultsRequest) error {
	if err := pbutil.ValidateTestResultPredicate(req.Predicate); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	return validateQueryRequest(req)
}

// QueryTestResults implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	if err := validateQueryTestResultsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Open a transaction.
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	if in.MaxStaleness != nil {
		st, _ := ptypes.Duration(in.MaxStaleness)
		txn.WithTimestampBound(spanner.MaxStaleness(st))
	}

	// Get the transitive closure.
	invs, err := span.ReadReachableInvocations(ctx, txn, maxInvocationGraphSize, span.MustParseInvocationNames(in.Invocations))
	if err != nil {
		return nil, err
	}

	// Query test results.
	trs, token, err := span.QueryTestResults(ctx, txn, span.TestResultQuery{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: invs,
	})
	if err != nil {
		return nil, err
	}

	if err := s.rewriteArtifactLinks(ctx, trs); err != nil {
		return nil, err
	}

	return &pb.QueryTestResultsResponse{
		NextPageToken: token,
		TestResults:   trs,
	}, nil
}

func (s *resultDBServer) rewriteArtifactLinks(ctx context.Context, trs []*pb.TestResult) error {
	for _, tr := range trs {
		for _, a := range tr.OutputArtifacts {
			// If the URL looks an isolate URL, then generate a signed plain HTTP
			// URL that serves the isolate file contents.
			if host, ns, digest, err := internal.ParseIsolateURL(a.FetchUrl); err == nil {
				u, exp, err := s.generateIsolateURL(ctx, host, ns, digest)
				if err != nil {
					return err
				}
				a.FetchUrl = u.String()
				a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
			}
		}
	}
	return nil
}
