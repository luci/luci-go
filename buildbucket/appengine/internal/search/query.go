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
	"context"
	"regexp"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/paged"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	defaultPageSize = 100
	maxPageSize     = 1000
)

var (
	TagIndexCursorRegex = regexp.MustCompile(`^id>\d+$`)
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

// IndexedTags returns the indexed tags.
func IndexedTags(tags strpair.Map) []string {
	set := make(stringset.Set)
	for k, vals := range tags {
		if k != "buildset" && k != "build_address" {
			continue
		}
		for _, val := range vals {
			set.Add(strpair.Format(k, val))
		}
	}
	return set.ToSortedSlice()
}

// Fetch performs main search builds logic.
func (q *Query) Fetch(ctx context.Context) (*pb.SearchBuildsResponse, error) {
	if !buildid.MayContainBuilds(q.StartTime, q.EndTime) {
		return &pb.SearchBuildsResponse{}, nil
	}

	// Validate bucket ACL permission.
	if q.Builder != nil && q.Builder.Bucket != "" {
		if err := perm.HasInBuilder(ctx, perm.BuildsList, q.Builder); err != nil {
			return nil, err
		}
	}

	// Determine which subflow - directly query on Builds or on TagIndex.
	isTagIndexCursor := TagIndexCursorRegex.MatchString(q.StartCursor)
	canUseTagIndex := len(IndexedTags(q.Tags)) != 0 && (q.StartCursor == "" || isTagIndexCursor)
	if canUseTagIndex {
		// TODO(crbug/1090540): test switch-case block after tagIndexSearch() is complete.
		switch res, err := q.tagIndexSearch(ctx); {
		case model.TagIndexIncomplete.In(err) && q.StartCursor == "":
			logging.Warningf(ctx, "Falling back to querying search on builds.")
		case err != nil:
			return nil, err
		default:
			return res, nil
		}
	}

	logging.Debugf(ctx, "Querying search on Build.")
	return q.fetchOnBuild(ctx)
}

// fetchOnBuild fetches directly on Build entity.
func (q *Query) fetchOnBuild(ctx context.Context) (*pb.SearchBuildsResponse, error) {
	// TODO(crbug/1090540): Fetch accessible buckets once the related function is implemented, if bucketId is nil.

	dq := datastore.NewQuery(model.BuildKind).Order("__key__")

	for _, tag := range q.Tags.Format() {
		dq = dq.Eq("tags", tag)
	}

	if q.Status != pb.Status_STATUS_UNSPECIFIED {
		dq = dq.Eq("status_v2", q.Status)
	}

	// TODO(crbug/1090540): filtering by created_by after it's been migrated to string.

	switch {
	case q.Builder.GetBuilder() != "":
		dq = dq.Eq("builder_id", protoutil.FormatBuilderID(q.Builder))
	case q.Builder.GetBucket() != "":
		dq = dq.Eq("bucket_id", protoutil.FormatBucketID(q.Builder.Project, q.Builder.Bucket))
	case q.Builder.GetProject() != "":
		dq = dq.Eq("project", q.Builder.Project)
	}

	idLow, idHigh := q.idRange()
	if idLow != nil {
		dq = dq.Gte("__key__", datastore.KeyForObj(ctx, &model.Build{ID: *idLow}))
	}
	if idHigh != nil {
		dq = dq.Lt("__key__", datastore.KeyForObj(ctx, &model.Build{ID: *idHigh}))
	}

	logging.Debugf(ctx, "datastore query for FetchOnBuild: %s", dq.String())
	rsp := &pb.SearchBuildsResponse{}
	err := paged.Query(ctx, q.PageSize, q.StartCursor, rsp, dq, func(build *model.Build) error {
		rsp.Builds = append(rsp.Builds, build.ToSimpleBuildProto(ctx))
		return nil
	})
	if err != nil {
		return nil, err
	}

	return rsp, nil
}

// TODO(crbug/1090540): implement search via tagIndex flow
func (q *Query) tagIndexSearch(ctx context.Context) (*pb.SearchBuildsResponse, error) {
	logging.Debugf(ctx, "Searching builds on TagIndex")
	return nil, nil
}

// idRange returns the id range from either q.BuildIdLow/q.BuildIdHigh or q.StartTime/q.EndTime.
func (q *Query) idRange() (*int64, *int64) {
	if q.BuildIdLow != nil || q.BuildIdHigh != nil {
		return q.BuildIdLow, q.BuildIdHigh
	}

	var idLow, idHigh *int64
	low, high := buildid.IdRange(q.StartTime, q.EndTime)
	if low != 0 {
		idLow = &low
	}
	if high != 0 {
		idHigh = &high
	}
	return idLow, idHigh
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
