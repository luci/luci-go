// Copyright 2020 The LUCI Authors.
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

// Package model in internal contains intermediate models
package model

import (
  "fmt"
  "go.chromium.org/luci/common/errors"
  "go.chromium.org/luci/grpc/appstatus"
  "time"

  "github.com/golang/protobuf/ptypes"

  "go.chromium.org/luci/buildbucket/appengine/internal/utils"
  pb "go.chromium.org/luci/buildbucket/proto"
  "go.chromium.org/luci/buildbucket/protoutil"

)

type SearchQuery struct {
  Project string
  BucketId string
  Builder string
  Tags []string
  Status  pb.Status
  CreatedBy string
  StartTime *time.Time
  EndTime *time.Time
  IncludeExperimental bool
  BuildHigh *int64 // O means no boundary
  BuildLow *int64 // O means no boundary
  Canary *bool
  PageSize int32
  StartCursor string
}

func NewSearchQuery(req *pb.SearchBuildsRequest) (*SearchQuery, error) {
  if req.Predicate == nil {
    return &SearchQuery{
      PageSize: req.PageSize,
      StartCursor: req.PageToken,
    }, nil
  }

  predicate := req.GetPredicate()
  tags := protoutil.ParseStringPairs(predicate.Tags)

  var project string
  var bucketId string
  var builder string
  if predicate.Builder != nil {
    bucketId = fmt.Sprintf("%s/%s", predicate.Builder.GetProject(), predicate.Builder.GetBucket())
    builder = predicate.Builder.GetBuilder()
  } else {
    project = predicate.Builder.GetProject()
  }

  for _, change := range predicate.GerritChanges {
    tags = append(tags, protoutil.GerritBuildSet(change))
  }

  var startTime time.Time
  var endTime time.Time
  startTime, err := ptypes.Timestamp(predicate.CreateTime.GetStartTime())
  if err != nil {
    return nil, appstatus.BadRequest(errors.Annotate(err, "CreateTime.StartTime").Err())
  }
  endTime, err = ptypes.Timestamp(predicate.CreateTime.GetEndTime())
  if err != nil {
    return nil, appstatus.BadRequest(errors.Annotate(err, "CreateTime.EndTime").Err())
  }

  var BuildHigh *int64
  var BuildLow *int64
  if predicate.Build.GetStartBuildId() > 0 {
    BuildHigh = utils.ToInt64Ptr(predicate.Build.GetStartBuildId() + 1)
  }
  if predicate.Build.GetEndBuildId() > 0 {
    BuildLow = utils.ToInt64Ptr(predicate.Build.GetEndBuildId() - 1)
  }

  var canary *bool
  if predicate.GetCanary() != pb.Trinary_UNSET {
    canary = utils.ToBoolPtr(predicate.GetCanary() == pb.Trinary_YES)
  }

  return &SearchQuery{
    Project:             project,
    BucketId:            bucketId,
    Builder:             builder,
    Tags:                tags,
    Status:              predicate.Status,
    CreatedBy:           predicate.CreatedBy,
    StartTime:           &startTime,
    EndTime:             &endTime,
    IncludeExperimental: predicate.GetIncludeExperimental(),
    BuildHigh:           BuildHigh,
    BuildLow:            BuildLow,
    Canary:              canary,
    PageSize:            req.GetPageSize(),
    StartCursor:         req.GetPageToken(),
  }, nil
}