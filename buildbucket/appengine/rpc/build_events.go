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

package rpc

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func buildCreated(ctx context.Context, b *model.Build) {
	var ua string
	for _, tag := range b.Tags {
		if k, v := strpair.Parse(tag); k == "user_agent" {
			ua = v
			break
		}
	}
	buildCountCreated.Add(ctx, 1, legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, ua)
}

func buildStarted(ctx context.Context, b *model.Build) {
	logging.Infof(ctx, "Build %d: started", b.ID)
	buildCountStarted.Add(
		ctx, 1,
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, b.Proto.Canary,
	)
	if b.Proto.GetStartTime() != nil {
		startT := b.Proto.StartTime.AsTime()
		buildDurationScheduling.Add(
			ctx, startT.Sub(b.CreateTime).Seconds(),
			legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, "", "", "", b.Proto.Canary,
		)
	}
}

func buildStarting(ctx context.Context, b *model.Build) error {
	return notifyPubSub(ctx, b)
}

func buildCompleted(ctx context.Context, b *model.Build) {
	r, fr, cr := getLegacyMetricFields(b)
	logging.Infof(ctx, "Build %d: completed by %q with status %q", b.ID, auth.CurrentIdentity(ctx), r)
	buildCountCompleted.Add(
		ctx, 1,
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary)

	endT := b.Proto.EndTime.AsTime()
	buildDurationCycle.Add(
		ctx, endT.Sub(b.CreateTime).Seconds(),
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary,
	)
	if b.Proto.StartTime != nil {
		startT := b.Proto.StartTime.AsTime()
		buildDurationRun.Add(
			ctx, endT.Sub(startT).Seconds(),
			legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary,
		)
	}
}

func buildCompleting(ctx context.Context, b *model.Build) error {
	bqTask := &taskdefs.ExportBigQuery{BuildId: b.ID}
	invTask := &taskdefs.FinalizeResultDB{BuildId: b.ID}
	return parallel.FanOutIn(func(tks chan<- func() error) {
		tks <- func() error { return notifyPubSub(ctx, b) }
		tks <- func() error { return tasks.ExportBigQuery(ctx, bqTask) }
		tks <- func() error { return tasks.FinalizeResultDB(ctx, invTask) }
	})
}

// legacyBucketName returns the V1 luci bucket name.
// e.g., "luci.chromium.try".
func legacyBucketName(bid *pb.BuilderID) string {
	return fmt.Sprintf("luci.%s.%s", bid.Project, bid.Bucket)
}
