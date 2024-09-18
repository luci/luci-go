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

package tasks

import (
	"context"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// PopPendingBuildTask is responsible for popping and triggering
// builds from the BuilderQueue pending_builds queue.
// PopPendingBuildTask runs at build termination for builder with Config.MaxConcurrentBuilds > 0,
// or at builder config ingestion for Config.MaxConcurrentBuilds increases and resets.
// A popped build is sent to task Backend and moved to the BuilderQueue triggered_builds set.
func PopPendingBuildTask(ctx context.Context, build int64, builder *pb.BuilderID) error {
	return errors.New("NotImplemented")
}
