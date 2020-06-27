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

package search

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	defaultPageSize = 100
	maxPageSize = 1000
)

// Query is the intermediate to store the arguments for ds search query.
type Query struct {
	Project             string
	BucketId            string
	Builder             string
	Tags                strpair.Map
	Status              pb.Status
	CreatedBy           identity.Identity
	StartTime           time.Time
	EndTime             time.Time
	IncludeExperimental bool
	BuildIdHigh         *int64
	BuildIdLow          *int64
	Canary              *bool
	PageSize            int32
	StartCursor         string
}

// NewQuery builds a search.Query from pb.SearchBuildsRequest.
func NewQuery(req *pb.SearchBuildsRequest) (*Query, error) {
	if req.GetPredicate() == nil {
		return &Query{
			PageSize:    req.GetPageSize(),
			StartCursor: req.GetPageToken(),
		}, nil
	}

	p := req.Predicate
	s := &Query{
		Project:             p.Builder.GetProject(),
		BucketId:            p.Builder.GetBucket(),
		Builder:             p.Builder.GetBuilder(),
		Tags:                protoutil.StringPairMap(p.Tags),
		Status:              p.Status,
		IncludeExperimental: p.IncludeExperimental,
		PageSize:            fixPageSize(req.PageSize),
		StartCursor:         req.PageToken,
	}

	var err error
	if p.CreatedBy != "" {
		if s.CreatedBy, err = identity.MakeIdentity(p.CreatedBy); err != nil {
			return nil, errors.Annotate(err, "CreateBy").Err()
		}
	}

	// Filter by gerrit changes.
	for _, change := range p.GerritChanges {
		s.Tags.Add("buildset", protoutil.GerritBuildSet(change))
	}

	// Filter by creation time.
	if s.StartTime, err = toTime(p.CreateTime.GetStartTime()); err != nil {
		return nil, errors.Annotate(err, "CreateTime.StartTime").Err()
	}
	if s.EndTime, err = toTime(p.CreateTime.GetEndTime()); err != nil {
		return nil, errors.Annotate(err, "CreateTime.EndTime" ).Err()
	}

	// Filter by build range.
	// BuildIds less or equal to 0 means no boundary.
	// Convert BuildRange to [buildLow, buildHigh).
	// Note that unlike buildLow/buildHigh, BuildRange in req encapsulates the fact
	// that build ids are decreasing. So we need to reverse the order.
	if p.Build.GetStartBuildId() > 0 {
		// Add 1 because startBuildId is inclusive and buildHigh is exclusive.
		s.BuildIdHigh = proto.Int64(p.Build.GetStartBuildId() + 1)
	}
	if p.Build.GetEndBuildId() > 0 {
		// Subtract 1 because endBuildId is exclusive and buildLow is inclusive.
		s.BuildIdLow = proto.Int64(p.Build.GetEndBuildId() - 1)
	}

	// Filter by canary.
	if p.GetCanary() != pb.Trinary_UNSET {
		s.Canary = proto.Bool(p.GetCanary() == pb.Trinary_YES)
	}
	return s, nil
}

// fixPageSize ensures the size is between the defaultPageSize and maxPageSize.
func fixPageSize(size int32) int32 {
	switch {
	case size <= 0:
		return defaultPageSize
	case size > maxPageSize:
		return maxPageSize
	default:
		return size
	}
}

// toTime converts a protobuf timestamp to a time.Time.
func toTime(t *timestamp.Timestamp) (time.Time, error) {
	if t == nil {
		return time.Time{}, nil
	}
  return ptypes.Timestamp(t)
}