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
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/paged"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/model"
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

func (q *Query) FetchOnBuilds(ctx context.Context, mask mask.Mask) (*pb.SearchBuildsResponse, error) {
	// TODO(crbug/1090540): Fetch accessible buckets once the related function is implemented, if bucketId is nil.

	logging.Debugf(ctx, "FetchOnBuilds...")
	dq := datastore.NewQuery(model.BuildKind).Order("__key__")
	// if tags := q.Tags.Format(); len(tags) != 0 {
	// 	fmt.Println("tags len ")
	// 	fmt.Println(len(tags))
	// 	dq = dq.Eq("tags", tags)
	// }
	for _, tag := range q.Tags.Format() {
		dq = dq.Eq("tags", tag)
	}

	if q.Status != pb.Status_STATUS_UNSPECIFIED {
		dq.Eq("status_v2", q.Status)
	}

	// TODO(crbug/1090540): make create_by indexed ???
	if q.CreatedBy != identity.Identity("") {
		dq = dq.Eq("created_by", []byte(q.CreatedBy))
	}

	switch {
	case q.Builder.GetBuilder() != "":
		dq = dq.Eq("builder_id", toBuilderId(q.Builder.GetProject(), q.Builder.GetBucket(), q.Builder.GetBuilder()))
	case q.Builder.GetBucket() != "":
		dq = dq.Eq("bucket_id", toBucketId(q.Builder.GetProject(), q.Builder.GetBucket()))
	case q.Builder.GetProject() != "":
		dq = dq.Eq("project", q.Builder.GetProject())
	}

	switch idLow, idHigh := buildid.IdRange(q.StartTime, q.EndTime); {
	case q.BuildIdLow != nil:
		dq = dq.Gte("__key__", datastore.KeyForObj(ctx, *q.BuildIdLow))
		fallthrough
	case q.BuildIdHigh != nil:
		dq = dq.Lt("__key__", datastore.KeyForObj(ctx, *q.BuildIdHigh))
	case idLow != 0:
		dq = dq.Gte("__key__", datastore.KeyForObj(ctx, idLow))
		fallthrough
	case idHigh != 0:
		dq = dq.Lt("__key__", datastore.KeyForObj(ctx, idHigh))
	}

	logging.Debugf(ctx, "datastore query %s", dq.String())
	fmt.Println("the query mask")
	fmt.Println(mask)
	rsp := &pb.SearchBuildsResponse{}
	if err := paged.Query(ctx, q.PageSize, q.StartCursor, rsp, dq, func(build *model.Build) error {
		pbBuild, ierr:= build.ToProto(ctx, mask)
		logging.Debugf(ctx, "pbBuild after toProto %s", pbBuild.String())
		if ierr != nil {
			return ierr
		}
		rsp.Builds = append(rsp.Builds, pbBuild)
		return nil
	}); err != nil {
		return nil, err
	}

	return rsp, nil
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

func toBuilderId(project, bucket, builder string) string {
	return fmt.Sprintf("%s/%s/%s", project, bucket, builder)
}

func toBucketId(project, bucket string) string {
	return fmt.Sprintf("%s/%s", project, bucket)
}