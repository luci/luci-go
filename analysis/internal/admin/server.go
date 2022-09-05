// Copyright 2022 The LUCI Authors.
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

package admin

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	adminpb "go.chromium.org/luci/analysis/internal/admin/proto"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/services/testvariantbqexporter"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// allowGroup is a Chrome Infra Auth group, members of which are allowed to call
// admin API.
const allowGroup = "service-luci-analysis-admins"

// adminServer implements adminpb.AdminServer.
type adminServer struct {
	adminpb.UnimplementedAdminServer
}

// CreateServer creates an adminServer.
func CreateServer() *adminServer {
	return &adminServer{}
}

func unspecified(field string) error {
	return fmt.Errorf("%s is not specified", field)
}

func bqExportFromConfig(ctx context.Context, realm, cloudProject, dataset, table string) (*configpb.BigQueryExport, error) {
	rc, err := config.Realm(ctx, realm)
	if err != nil {
		return nil, err
	}
	for _, bqexport := range rc.GetTestVariantAnalysis().GetBqExports() {
		tableC := bqexport.GetTable()
		if tableC == nil {
			continue
		}
		if tableC.GetCloudProject() == cloudProject && tableC.GetDataset() == dataset && tableC.GetTable() == table {
			// The table in request is found in config.
			return bqexport, nil
		}
	}
	return nil, fmt.Errorf("table not found in realm config")
}

func validateTable(ctx context.Context, realm, cloudProject, dataset, table string) error {
	switch {
	case cloudProject == "":
		return unspecified("cloud project")
	case dataset == "":
		return unspecified("dataset")
	case table == "":
		return unspecified("table")
	}

	_, err := bqExportFromConfig(ctx, realm, cloudProject, dataset, table)
	return err
}

func validateTimeRange(ctx context.Context, timeRange *pb.TimeRange) error {
	switch {
	case timeRange.GetEarliest() == nil:
		return unspecified("timeRange.Earliest")
	case timeRange.GetLatest() == nil:
		return unspecified("timeRange.Latest")
	}

	earliest, err := pbutil.AsTime(timeRange.Earliest)
	if err != nil {
		return err
	}

	latest, err := pbutil.AsTime(timeRange.Latest)
	if err != nil {
		return err
	}

	if !earliest.Before(latest) {
		return fmt.Errorf("timeRange: earliest must be before latest")
	}

	if !latest.Before(clock.Now(ctx)) {
		return fmt.Errorf("timeRange: latest must not be in the future")
	}
	return nil
}

func validateExportTestVariantsRequest(ctx context.Context, req *adminpb.ExportTestVariantsRequest) error {
	if req.GetRealm() == "" {
		return unspecified("realm")
	}
	if err := validateTable(ctx, req.Realm, req.GetCloudProject(), req.GetDataset(), req.GetTable()); err != nil {
		return err
	}

	return validateTimeRange(ctx, req.GetTimeRange())
}

type rangeInTime struct {
	start time.Time
	end   time.Time
}

// splitTimeRange split the given time range to a slice of smaller ranges,
// each is testvariantbqexporter.BqExportJobInterval long.
func splitTimeRange(timeRange *pb.TimeRange) ([]rangeInTime, error) {
	earliest, err := pbutil.AsTime(timeRange.Earliest)
	if err != nil {
		return nil, err
	}
	latest, err := pbutil.AsTime(timeRange.Latest)
	if err != nil {
		return nil, err
	}

	// Truncate both ends to full hour.
	earliest = earliest.Truncate(time.Hour)
	latest = latest.Truncate(time.Hour)

	// Split the time range to a slice of sub ranges - each is BqExportJobInterval long.
	delta := latest.Sub(earliest)
	subRangeNum := int(delta / testvariantbqexporter.BqExportJobInterval)
	ranges := make([]rangeInTime, subRangeNum)
	start := earliest
	for i := 0; i < subRangeNum; i++ {
		end := start.Add(testvariantbqexporter.BqExportJobInterval)
		if end.After(latest) {
			end = latest
		}
		ranges[i] = rangeInTime{
			start: start,
			end:   end,
		}
		start = end
	}
	return ranges, nil
}

// ExportTestVariants implements AdminServer.
func (a *adminServer) ExportTestVariants(ctx context.Context, req *adminpb.ExportTestVariantsRequest) (*emptypb.Empty, error) {
	if err := checkAllowed(ctx, "ExportTestVariants"); err != nil {
		return nil, err
	}

	if err := validateExportTestVariantsRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	subRanges, err := splitTimeRange(req.TimeRange)
	if err != nil {
		return nil, err
	}

	bqExport, err := bqExportFromConfig(ctx, req.Realm, req.CloudProject, req.Dataset, req.Table)
	if err != nil {
		return nil, err
	}

	for _, r := range subRanges {
		err := testvariantbqexporter.Schedule(ctx, req.Realm, req.CloudProject, req.Dataset, req.Table, bqExport.GetPredicate(), &pb.TimeRange{
			Earliest: timestamppb.New(r.start),
			Latest:   timestamppb.New(r.end),
		})
		if err != nil {
			return nil, err
		}
	}

	return &emptypb.Empty{}, nil
}

func checkAllowed(ctx context.Context, name string) error {
	switch yes, err := auth.IsMember(ctx, allowGroup); {
	case err != nil:
		return errors.Annotate(err, "failed to check ACL").Err()
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", allowGroup)
	default:
		logging.Warningf(ctx, "%s is calling admin.%s", auth.CurrentIdentity(ctx), name)
		return nil
	}
}
