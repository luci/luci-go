// Copyright 2024 The LUCI Authors.
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

package rpc

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/common/buildcel"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var cmprWalker = protowalk.NewWalker[*pb.CustomMetricPreviewRequest](protowalk.RequiredProcessor{})

func validateCustomMetricPreviewRequest(ctx context.Context, req *pb.CustomMetricPreviewRequest) error {
	if procRes := cmprWalker.Execute(req); !procRes.Empty() {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		if err := procRes.Err(); err != nil {
			return err
		}
	}
	if len(req.MetricDefinition.Predicates) == 0 {
		return errors.Reason("metric_definition.predicates is required").Err()
	}

	if req.GetMetricBase() == pb.CustomMetricBase_CUSTOM_METRIC_BASE_UNSET {
		return errors.Reason("metric_base is required").Err()
	}

	var extraFields []string
	for f := range req.MetricDefinition.ExtraFields {
		extraFields = append(extraFields, f)
	}
	return metrics.ValidateExtraFieldsWithBase(req.GetMetricBase(), extraFields)
}

// CustomMetricPreview evaluates a build with a custom metric definition and returns the preview result. Implements pb.BuildsServer.
func (*Builds) CustomMetricPreview(ctx context.Context, req *pb.CustomMetricPreviewRequest) (*pb.CustomMetricPreviewResponse, error) {
	if err := validateCustomMetricPreviewRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	bld, err := common.GetBuild(ctx, req.BuildId)
	if err != nil {
		return nil, err
	}
	// Preview needs to read everything from the build, so require the caller
	// has the permission to get the whole build.
	if err := perm.HasInBuilder(ctx, bbperms.BuildsGet, bld.Proto.Builder); err != nil {
		return nil, err
	}

	// Get build details.
	m, err := model.NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
	if err != nil {
		return nil, err
	}
	bp, err := bld.ToProto(ctx, m, func(b *pb.Build) error {
		return nil
	})
	if err != nil {
		return nil, err
	}

	res := &pb.CustomMetricPreviewResponse{}
	// Evaluate predicates.
	matched, err := buildcel.BoolEval(bp, req.MetricDefinition.Predicates)
	switch {
	case err != nil:
		res.Response = &pb.CustomMetricPreviewResponse_Error{
			Error: fmt.Sprintf("failed to evaluate the build with predicates: %s", err),
		}
		return res, nil
	case !matched:
		res.Response = &pb.CustomMetricPreviewResponse_Error{
			Error: "the build doesn't pass the predicates evaluation, it will not be reported by the custom metric",
		}
		return res, nil
	}

	// Evaluate extra_fields.
	baseFieldMap, err := metrics.GetBaseFieldMap(bp, req.GetMetricBase())
	if err != nil {
		return nil, err
	}
	var extraFieldMap map[string]string
	if len(req.MetricDefinition.ExtraFields) > 0 {
		extraFieldMap, err = buildcel.StringMapEval(bp, req.MetricDefinition.ExtraFields)
		if err != nil {
			res.Response = &pb.CustomMetricPreviewResponse_Error{
				Error: fmt.Sprintf("failed to evaluate the build with extra_fields: %s", err),
			}
			return res, nil
		}
	}
	fields := make([]*pb.StringPair, 0, len(baseFieldMap)+len(extraFieldMap))
	for f, v := range baseFieldMap {
		fields = append(fields, &pb.StringPair{Key: f, Value: v})
	}
	for f, v := range extraFieldMap {
		fields = append(fields, &pb.StringPair{Key: f, Value: v})
	}
	res.Response = &pb.CustomMetricPreviewResponse_Report_{
		Report: &pb.CustomMetricPreviewResponse_Report{
			Fields: fields,
		},
	}
	return res, nil
}
