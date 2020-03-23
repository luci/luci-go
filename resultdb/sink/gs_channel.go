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
package sink

import (
	"context"
	"sort"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type uploadBucket struct {
	upperLimit int64
	channel    *dispatcher.Channel
}

type gsChannel struct {
	buckets []*uploadBucket
}

func (gsc *gsChannel) init(ctx context.Context) error {
	// TODO(ddoman): add tsmon metrics to insepct and monitor upload buckets, and tune
	// the dispatcher options, if necessary.
	if err := gsc.addUploadBucket(
		ctx,
		// artifacts <= 2kb in size
		2*1024,
		buffer.Options{
			BatchSize:     128,
			MaxLeases:     2,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 1024},
		},
	); err != nil {
		return err
	}

	if err := gsc.addUploadBucket(
		ctx,
		// artifacts <= 32kb in size
		32*1024,
		buffer.Options{
			BatchSize:     16,
			MaxLeases:     2,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 1024},
		},
	); err != nil {
		return err
	}

	if err := gsc.addUploadBucket(
		ctx,
		// artifacts <= 256kb
		256*1024,
		buffer.Options{
			BatchSize:     2,
			MaxLeases:     4,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 512},
		},
	); err != nil {
		return err
	}

	if err := gsc.addUploadBucket(
		ctx,
		// This bucket has the largest upper limit, and any artifacts bigger than the limit
		// are added into this bucket.
		512*1024,
		buffer.Options{
			BatchSize:     1,
			MaxLeases:     4,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 512},
		},
	); err != nil {
		return err
	}
	return nil
}

func (gsc *gsChannel) closeAndDrain(ctx context.Context) {
	var wg sync.WaitGroup
	for _, bucket := range gsc.buckets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bucket.channel.CloseAndDrain(ctx)
		}()
	}
	// channel.CloseAndDrain returns immediately if the context is cancelled.
	wg.Wait()
}

func (gsc *gsChannel) addUploadBucket(ctx context.Context, upperLimit int64, bufOpts buffer.Options) error {
	// panic if there is an existing bucket with the same upper limit.
	if len(gsc.buckets) > 0 {
		ub := gsc.searchUploadBucket(upperLimit)
		if ub.upperLimit == upperLimit {
			panic("cannot add an uploadBucket with a duplicate upper limit")
		}
	}
	ch, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer:   bufOpts,
	}, func(b *buffer.Batch) error {
		// TODO(ddoman): upload artifacts to a Google Storage bucket
		return nil
	})
	if err != nil {
		return err
	}
	gsc.buckets = append(gsc.buckets, &uploadBucket{upperLimit, &ch})
	sort.SliceStable(gsc.buckets, func(i, j int) bool {
		return gsc.buckets[i].upperLimit < gsc.buckets[j].upperLimit
	})
	return nil
}

func (gsc *gsChannel) searchUploadBucket(size int64) *uploadBucket {
	if len(gsc.buckets) == 0 {
		panic("there must be at least one upload bucket")
	}
	i := sort.Search(len(gsc.buckets), func(i int) bool {
		return gsc.buckets[i].upperLimit >= size
	})
	// use the bucket with the largest upper limit, if no bucket was found.
	if i == len(gsc.buckets) {
		return gsc.buckets[len(gsc.buckets)-1]
	}
	return gsc.buckets[i]
}

func (gsc *gsChannel) uploadArtifact(size int64, art *sinkpb.Artifact) {
	bucket := gsc.searchUploadBucket(size)
	bucket.channel.C <- art
}
