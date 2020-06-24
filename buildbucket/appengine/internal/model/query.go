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
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/buildbucket/appengine/internal/utils"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
)

// SearchQuery is the intermediate to store the arguments for ds search query.
type SearchQuery struct {
	Project             string
	BucketId            string
	Builder             string
	Tags                []string
	Status              pb.Status
	CreatedBy           string
	StartTime           *time.Time
	EndTime             *time.Time
	IncludeExperimental bool
	BuildHigh           *int64
	BuildLow            *int64
	Canary              *bool
	PageSize            int32
	StartCursor         string
}

// NewSearchQuery builds a SearchQuery from pb.SearchBuildsRequest.
func NewSearchQuery(req *pb.SearchBuildsRequest) (*SearchQuery, error) {
	if req.GetPredicate() == nil {
		return &SearchQuery{
			PageSize:    req.GetPageSize(),
			StartCursor: req.GetPageToken(),
		}, nil
	}

	p := req.Predicate
	s := &SearchQuery{
		Status:              p.Status,
		CreatedBy:           p.CreatedBy,
		IncludeExperimental: p.GetIncludeExperimental(),
		PageSize:            req.PageSize,
		StartCursor:         req.PageToken,
	}
	s.Tags = protoutil.ParseStringPairs(p.Tags)

	// Filter by builder.
	if p.Builder != nil {
		s.BucketId = fmt.Sprintf("%s/%s", p.Builder.GetProject(), p.Builder.GetBucket())
		s.Builder = p.Builder.GetBuilder()
	} else {
		s.Project = p.Builder.GetProject()
	}

	// Filter by gerrit changes.
	for _, change := range p.GerritChanges {
		s.Tags = append(s.Tags, protoutil.GerritBuildSet(change))
	}

	// Filter by creation time.
	var startTime time.Time
	var endTime time.Time
	startTime, err := ptypes.Timestamp(p.CreateTime.GetStartTime())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "CreateTime.StartTime").Err())
	}
	s.StartTime = &startTime
	endTime, err = ptypes.Timestamp(p.CreateTime.GetEndTime())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "CreateTime.EndTime").Err())
	}
	s.EndTime = &endTime

	// Filter by build range.
	// BuildIds less or equal to 0 means no boundary.
	// Convert BuildRange to [buildLow, buildHigh).
	// Note that unlike buildLow/buildHigh, BuildRange in req encapsulates the fact
	// that build ids are decreasing. So we need to reverse the order.
	if p.Build.GetStartBuildId() > 0 {
		// Add 1 because startBuildId is inclusive and buildHigh is exclusive.
		s.BuildHigh = utils.ToInt64Ptr(p.Build.GetStartBuildId() + 1)
	}
	if p.Build.GetEndBuildId() > 0 {
		// Subtract 1 because endBuildId is exclusive and buildLow is inclusive.
		s.BuildLow = utils.ToInt64Ptr(p.Build.GetEndBuildId() - 1)
	}

	// Filter by canary.
	if p.GetCanary() != pb.Trinary_UNSET {
		s.Canary = utils.ToBoolPtr(p.GetCanary() == pb.Trinary_YES)
	}
	return s, nil
}