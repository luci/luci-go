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

// PushPendingBuildTask is responsible for updating the BuilderQueue entity.
// It is triggered at the build creation for builder with Config.MaxConcurrentBuilds > 0.
// A new build can either be pushed to triggered_builds and sent to task Backend, or
// pushed to pending_builds where it will wait to be popped when the builder gets some capacity.
func PushPendingBuildTask(ctx context.Context, build int64, builder *pb.BuilderID) error {
	return errors.New("NotImplemented")
}
