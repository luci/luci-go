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

package prjmanager

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
	"go.chromium.org/luci/gae/service/datastore"
)

// UpdateConfig tells ProjectManager to read and update to newest ProjectConfig
// by fetching it from Datatstore.
//
// Results in stopping ProjectManager if ProjectConfig got disabled or deleted.
func UpdateConfig(ctx context.Context, luciProject string) error {
	return send(ctx, luciProject, &internal.Event{
		Event: &internal.Event_UpdateConfig{
			UpdateConfig: &internal.UpdateConfig{},
		},
	})
}

// Poke tells ProjectManager to poke all downstream actors and check its own
// state.
func Poke(ctx context.Context, luciProject string) error {
	return send(ctx, luciProject, &internal.Event{
		Event: &internal.Event_Poke{
			Poke: &internal.Poke{},
		},
	})
}

func send(ctx context.Context, luciProject string, e *internal.Event) error {
	value, err := proto.Marshal(e)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	to := datastore.MakeKey(ctx, ProjectKind, luciProject)
	if err := eventbox.Emit(ctx, value, to); err != nil {
		return err
	}
	return internal.Dispatch(ctx, luciProject, time.Time{} /*asap*/)
}
