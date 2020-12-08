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

package impl

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

func init() {
	internal.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokePMTask)
			switch err := pokePMTask(ctx, task.GetLuciProject()); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				return tq.Fatal.Apply(err)
			default:
				// TODO(tandrii): avoid retries iff we know a new task was already
				// scheduled for the next second.
				return err
			}
		},
	)
}

const maxEventsToProcess = 256

func pokePMTask(ctx context.Context, luciProject string) error {
	// Read events outside of transactions.
	events, err := internal.Peek(ctx, luciProject, maxEventsToProcess)
	if err != nil {
		return err
	}
	// TODO(tandrii): use the events.
	return internal.Delete(ctx, events)
}
