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

package buildbucket

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/structmask"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// SubscriptionID is the default subscription ID for listening to Buildbucket
// build updates.
const SubscriptionID = "buildbucket-builds"

// Backend implements updaterBackend and cancellatorBackend interfaces.
//
// It can talk to Buildbucket to get Tryjobs' details and to cancel them.
type Backend struct {
	ClientFactory ClientFactory
}

func (b *Backend) Kind() string {
	return "buildbucket"
}

var TryjobBuildMask = &bbpb.BuildMask{
	Fields: &fieldmaskpb.FieldMask{
		Paths: []string{"id", "status", "output.properties", "create_time", "update_time"},
	},
	OutputProperties: []*structmask.StructMask{
		// Legacy.
		{Path: []string{"do_not_retry"}},
		{Path: []string{"failure_type"}},
		{Path: []string{"triggered_build_ids"}},
		// New protobuf-based property.
		{Path: []string{"$recipe_engine/cq/output"}},
	},
}

// Update retrieves the Buildbucket build corresponding to the given Tryjob,
// parses its output and returns its current Status and Result.
//
// It does not modify the given Tryjob.
func (b *Backend) Update(ctx context.Context, saved *tryjob.Tryjob) (tryjob.Status, *tryjob.Result, error) {
	host, buildID, err := saved.ExternalID.ParseBuildbucketID()
	if err != nil {
		return 0, nil, err
	}

	bbClient, err := b.ClientFactory.MakeClient(ctx, host, saved.LUCIProject())
	if err != nil {
		return 0, nil, err
	}

	build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID, Mask: TryjobBuildMask})
	switch code := status.Code(err); {
	case code == codes.OK:
		return toTryjobStatusAndResult(ctx, build)
	case grpcutil.IsTransientCode(code) || code == codes.DeadlineExceeded:
		return 0, nil, transient.Tag.Apply(err)
	default:
		return 0, nil, err
	}
}

// CancelTryjob asks buildbucket to cancel a running tryjob.
//
// It returns nil error if the buildbucket build is ended.
func (b *Backend) CancelTryjob(ctx context.Context, tj *tryjob.Tryjob) error {
	host, buildID, err := tj.ExternalID.ParseBuildbucketID()
	if err != nil {
		return err
	}

	bbClient, err := b.ClientFactory.MakeClient(ctx, host, tj.LUCIProject())
	if err != nil {
		return err
	}

	_, err = bbClient.CancelBuild(ctx, &bbpb.CancelBuildRequest{
		Id: buildID,
	})
	return err
}

func toTryjobStatusAndResult(ctx context.Context, b *bbpb.Build) (tryjob.Status, *tryjob.Result, error) {
	s := tryjob.Status_STATUS_UNSPECIFIED
	r := &tryjob.Result{
		CreateTime: b.CreateTime,
		UpdateTime: b.UpdateTime,
		Backend: &tryjob.Result_Buildbucket_{
			Buildbucket: &tryjob.Result_Buildbucket{
				Id:              b.Id,
				Status:          b.Status,
				SummaryMarkdown: b.SummaryMarkdown,
			},
		},
	}

	buildResult := parseBuildResult(ctx, b)
	r.Output = buildResult.output
	if buildResult.error != nil {
		logging.Debugf(ctx, "errors parsing recipe output: %s", buildResult.error)
		if buildResult.error.WithSeverity(validation.Blocking) != nil {
			r.Output = &recipe.Output{}
			logging.Debugf(ctx, "ignoring recipe output due to blocking parsing errors")
		}
	}

	switch b.Status {
	case bbpb.Status_FAILURE:
		s = tryjob.Status_ENDED
		if buildResult.isTransFailure {
			r.Status = tryjob.Result_FAILED_TRANSIENTLY
		} else {
			r.Status = tryjob.Result_FAILED_PERMANENTLY
		}
	case bbpb.Status_INFRA_FAILURE, bbpb.Status_CANCELED:
		s = tryjob.Status_ENDED
		r.Status = tryjob.Result_FAILED_TRANSIENTLY
	case bbpb.Status_SUCCESS:
		s = tryjob.Status_ENDED
		r.Status = tryjob.Result_SUCCEEDED
	case bbpb.Status_STARTED, bbpb.Status_SCHEDULED:
		s = tryjob.Status_TRIGGERED
		r.Status = tryjob.Result_UNKNOWN
	default:
		return s, nil, errors.Reason("unexpected buildbucket status %q", b.Status).Err()
	}
	return s, r, nil
}
