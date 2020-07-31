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

package finalizer

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
)

func init() {
	tasks.Finalization.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.TryFinalizeInvocation)
		logging.Infof(ctx, "Asked to finalize %s", task.InvocationId)
		return errors.New("not implemented yet")
	})
}
