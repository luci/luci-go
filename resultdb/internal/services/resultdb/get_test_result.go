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

package resultdb

import (
	"context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func validateGetTestResultRequest(req *pb.GetTestResultRequest) error {
	if err := pbutil.ValidateTestResultName(req.Name); err != nil {
		return errors.Annotate(err, "name").Err()
	}

	return nil
}

func (s *resultDBServer) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
	if err := validateGetTestResultRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	tr, err := span.ReadTestResult(ctx, txn, in.Name)
	if err != nil {
		return nil, err
	}

	if err := s.rewriteArtifactLinks(ctx, tr); err != nil {
		return nil, err
	}

	return tr, nil
}

func (s *resultDBServer) rewriteArtifactLinks(ctx context.Context, trs ...*pb.TestResult) error {
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
