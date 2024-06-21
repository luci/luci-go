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

// Package admin contains RPCs used to manage the LUCI Analysis service.
package admin

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/analysis/internal/admin/proto"
	"go.chromium.org/luci/analysis/internal/services/backfill"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

type adminServer struct{}

func NewAdminServer() *pb.DecoratedAdmin {
	return &pb.DecoratedAdmin{
		Prelude:  checkAllowedPrelude,
		Service:  &adminServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

func (*adminServer) BackfillTestResults(ctx context.Context, req *pb.BackfillTestResultsRequest) (*pb.BackfillTestResultsResponse, error) {
	if err := validateBackfillTestResultsRequest(req); err != nil {
		return nil, appstatus.Attachf(err, codes.InvalidArgument, "%s", err)
	}

	var count int
	for day := req.StartDay.AsTime(); day.Before(req.EndDay.AsTime()); day = day.Add(24 * time.Hour) {
		task := &taskspb.Backfill{
			Day: timestamppb.New(day),
		}
		if err := backfill.Schedule(ctx, task); err != nil {
			return nil, errors.Annotate(err, "schedule backfill for day %v", day).Err()
		}
		count++
	}
	return &pb.BackfillTestResultsResponse{
		DaysScheduled: int32(count),
	}, nil
}

func validateBackfillTestResultsRequest(req *pb.BackfillTestResultsRequest) error {
	startDay := req.StartDay.AsTime()
	if !startDay.Equal(startDay.Truncate(24 * time.Hour)) {
		return errors.Reason("start_day: must be the start of a day").Err()
	}
	endDay := req.EndDay.AsTime()
	if !endDay.Equal(endDay.Truncate(24 * time.Hour)) {
		return errors.Reason("end_day: must be the start of a day").Err()
	}
	if endDay.Before(startDay) {
		return errors.Reason("end_day: must be equal to or after start_day").Err()
	}
	return nil
}
