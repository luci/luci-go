// Copyright 2025 The LUCI Authors.
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
	"sync"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// bucketsMap holds a request-scoped cache of buckets.
type bucketsMap struct {
	m       sync.RWMutex
	buckets map[string]*model.Bucket // nil value means the bucket is missing
}

// prefetchBuckets prefetches all buckets mentioned in `reqs`.
//
// This is called before full request validation and it thus defensively skips
// requests without bucket reference.
//
// Returns an error only if the datastore fetch fails in unexpected way. Missing
// buckets are not an error.
func prefetchBuckets(ctx context.Context, reqs []*pb.ScheduleBuildRequest) (*bucketsMap, error) {
	var buckets []*model.Bucket
	bucketByID := map[string]*model.Bucket{}

	for _, req := range reqs {
		builderID := req.GetBuilder()
		if protoutil.ValidateRequiredBuilderID(builderID) != nil {
			continue
		}
		bucketID := protoutil.FormatBucketID(builderID.Project, builderID.Bucket)
		if _, ok := bucketByID[bucketID]; !ok {
			bucketByID[bucketID] = nil
			buckets = append(buckets, &model.Bucket{
				Parent: model.ProjectKey(ctx, builderID.Project),
				ID:     builderID.Bucket,
			})
		}
	}

	err := datastore.Get(ctx, buckets)
	var merr errors.MultiError
	if err != nil && !errors.As(err, &merr) {
		return nil, appstatus.Errorf(codes.Internal, "failed to fetch bucket configs: %s", err)
	}

	bucketErr := func(idx int) error {
		if merr == nil {
			return nil
		}
		return merr[idx]
	}

	// Store actually fetched buckets into `bucketByID`, leaving nil entries for
	// not found buckets.
	for i, bucket := range buckets {
		bucketID := protoutil.FormatBucketID(bucket.Parent.StringID(), bucket.ID)
		switch err := bucketErr(i); {
		case err == nil && bucket.Proto != nil:
			bucketByID[bucketID] = bucket
		case err == nil && bucket.Proto == nil:
			return nil, appstatus.Errorf(codes.Internal, "bucket %q entity is malformed: no bucket proto", bucketID)
		case !errors.Is(err, datastore.ErrNoSuchEntity):
			return nil, appstatus.Errorf(codes.Internal, "failed to fetch bucket configs of %q: %s", bucketID, err)
		}
	}

	return &bucketsMap{buckets: bucketByID}, nil
}

// Prefetch fetches the bucket "<project>/<bucket>" if not already fetched.
//
// Returns an error only if the datastore fetch fails in unexpected way. If the
// datastore operation succeeds, updates the map with the state of the fetched
// bucket (whether it exists or not)
func (m *bucketsMap) Prefetch(ctx context.Context, id string) error {
	m.m.RLock()
	_, ok := m.buckets[id]
	m.m.RUnlock()
	if ok {
		return nil
	}

	m.m.Lock()
	defer m.m.Unlock()
	if _, ok := m.buckets[id]; ok {
		return nil
	}

	projectID, bucketID, err := protoutil.ParseBucketID(id)
	if err != nil {
		return appstatus.Errorf(codes.Internal, "unexpected bucket ID format %q", id)
	}

	bucket := &model.Bucket{
		Parent: model.ProjectKey(ctx, projectID),
		ID:     bucketID,
	}
	switch err := datastore.Get(ctx, bucket); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		m.buckets[id] = nil
	case err == nil && bucket.Proto != nil:
		m.buckets[id] = bucket
	case err == nil && bucket.Proto == nil:
		return appstatus.Errorf(codes.Internal, "failed to fetch bucket %q config: no proto config", id)
	default:
		return appstatus.Errorf(codes.Internal, "failed to fetch bucket %q config: %s", id, err)
	}

	return nil
}

// Bucket returns a prefetched bucket or panics.
//
// If the bucket was prefetched, but it doesn't actually exist, returns nil.
func (m *bucketsMap) Bucket(id string) *pb.Bucket {
	m.m.RLock()
	defer m.m.RUnlock()
	switch b, ok := m.buckets[id]; {
	case !ok:
		panic(fmt.Sprintf("bucket %q wasn't prefetched", id))
	case b == nil:
		return nil
	default:
		return b.Proto
	}
}

// ShadowBucket returns a shadow bucket used by the given bucket.
//
// The shadow bucket is returned using its bucket name without "<project>/".
// Shadow buckets are always in the same project as the buckets they are
// shadowing.
//
// Returns an empty string if the given bucket doesn't exist or doesn't have
// a shadow bucket. Panics if used with a bucket that hasn't been prefetched.
func (m *bucketsMap) ShadowBucket(id string) string {
	m.m.RLock()
	defer m.m.RUnlock()
	b, ok := m.buckets[id]
	if !ok {
		panic(fmt.Sprintf("bucket %q wasn't prefetched", id))
	}
	if b == nil {
		return ""
	}
	return b.Proto.GetShadow()
}
