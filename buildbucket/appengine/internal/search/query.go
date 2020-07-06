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

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	defaultPageSize = 100
	maxPageSize = 1000
)

// Query is the intermediate to store the arguments for ds search query.
type Query struct {
	Builder             *pb.BuilderID
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

// NewQuery builds a Query from pb.SearchBuildsRequest.
// It assumes CreateTime in req is either unset or valid and will panic on any failures.
func NewQuery(req *pb.SearchBuildsRequest) *Query {
	if req.GetPredicate() == nil {
		return &Query{
			PageSize:    req.GetPageSize(),
			StartCursor: req.GetPageToken(),
		}
	}

	p := req.Predicate
	s := &Query{
		Builder:             p.GetBuilder(),
		Tags:                protoutil.StringPairMap(p.Tags),
		Status:              p.Status,
		CreatedBy:           identity.Identity(p.CreatedBy),
		StartTime:           mustTimestamp(p.CreateTime.GetStartTime()),
		EndTime:             mustTimestamp(p.CreateTime.GetEndTime()),
		IncludeExperimental: p.IncludeExperimental,
		PageSize:            fixPageSize(req.PageSize),
		StartCursor:         req.PageToken,
	}

	// Filter by gerrit changes.
	for _, change := range p.GerritChanges {
		s.Tags.Add("buildset", protoutil.GerritBuildSet(change))
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
	return s
}

// fixPageSize ensures the size is positive and less than or equal to maxPageSize.
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

// mustTimestamp converts a protobuf timestamp to a time.Time and panics on failures.
// It returns zero time for nil timestamp.
func mustTimestamp(ts *timestamp.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}

	t, err := ptypes.Timestamp(ts)
	if err != nil {
		panic(err)
	}
	return t
}