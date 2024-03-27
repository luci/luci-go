// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"context"
	"math"

	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	metricCount = metric.NewCounter(
		"cv/gerrit_rpc/count",
		"Total number of RPCs.",
		nil,

		field.String("luci_project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // status.Code of the result as string in UPPER_CASE.
	)

	metricDurationMS = metric.NewCumulativeDistribution(
		"cv/gerrit_rpc/duration",
		"Distribution of RPC duration (in milliseconds).",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms..10m range since CV isn't going to wait longer than 10m
		// anyway.
		//
		// $ python3 -c "print(((10**0.058)**100)/1e3/60.0)"
		// 10.515955741336601
		distribution.GeometricBucketer(math.Pow(10, 0.058), 100),

		field.String("luci_project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // status.Code of the result as string in UPPER_CASE.
	)
)

// InstrumentedFactory instruments RPCs.
func InstrumentedFactory(f Factory) Factory {
	return instrumentedFactory{Factory: f}
}

type instrumentedFactory struct {
	Factory
}

// MakeClient implements Factory.
func (i instrumentedFactory) MakeClient(ctx context.Context, gerritHost string, luciProject string) (Client, error) {
	c, err := i.Factory.MakeClient(ctx, gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	return instrumentedClient{
		luciProject: luciProject,
		gerritHost:  gerritHost,
		actual:      c,
	}, nil
}

type instrumentedClient struct {
	luciProject string
	gerritHost  string
	actual      Client
}

// start records the start time and returns the func to report the end.
func (i instrumentedClient) start(ctx context.Context, method string) func(err error) error {
	tStart := clock.Now(ctx)
	return func(err error) error {
		dur := clock.Since(ctx, tStart)

		// gerrit client we use is known to sometimes wrap grpc errors.
		c := status.Code(errors.Unwrap(err))
		canonicalCode, ok := code.Code_name[int32(c)]
		if !ok {
			canonicalCode = c.String() // Code(%d)
		}

		metricCount.Add(ctx, 1, i.luciProject, i.gerritHost, method, canonicalCode)
		metricDurationMS.Add(ctx, dur.Seconds()*1e3, i.luciProject, i.gerritHost, method, canonicalCode)

		return err
	}
}

func (i instrumentedClient) ListAccountEmails(ctx context.Context, req *gerritpb.ListAccountEmailsRequest, opts ...grpc.CallOption) (*gerritpb.ListAccountEmailsResponse, error) {
	end := i.start(ctx, "ListAccountEmails")
	resp, err := i.actual.ListAccountEmails(ctx, req, opts...)
	return resp, end(err)
}

func (i instrumentedClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	end := i.start(ctx, "ListChanges")
	resp, err := i.actual.ListChanges(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	end := i.start(ctx, "GetChange")
	resp, err := i.actual.GetChange(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	end := i.start(ctx, "GetRelatedChanges")
	resp, err := i.actual.GetRelatedChanges(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	end := i.start(ctx, "ListFiles")
	resp, err := i.actual.ListFiles(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	end := i.start(ctx, "SetReview")
	resp, err := i.actual.SetReview(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error) {
	end := i.start(ctx, "SubmitRevision")
	resp, err := i.actual.SubmitRevision(ctx, in, opts...)
	return resp, end(err)
}
