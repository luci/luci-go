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

package spanner

import (
	"context"

	gspan "cloud.google.com/go/spanner"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type Client struct {
	// base specifies the underlying Spanner client (with the database to which to talk).
	base *gspan.Client

	// lastMutationsBatch contains the data that was converted into mutations in the last transaction.
	// This is especially useful for testing.
	lastMutationsBatch []*mutationMetadata
}

// mutationMetadata logs the metadata of a transaction writing into a table.
type mutationMetadata struct {
	table string
	cols  []string
	vals  [][]interface{}
}

// GetInvocation returns the invocation with the given invID.
func (cl *Client) GetInvocation(ctx context.Context, invID string) (*pb.Invocation, error) {
	// TODO(jchinlee): implement
	return nil, nil
}

// WriteTestResults writes the given Invocation and TestResults in one read-write transaction.
func (cl *Client) WriteTestResults(ctx context.Context, invID string, inv *pb.Invocation, results []*pb.TestResult) error {
	// TODO(jchinlee): implement
	return nil
}
