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

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/dsset"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
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

func send(ctx context.Context, luciProject string, e *internal.Event) error {
	value, err := proto.Marshal(e)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	err = internal.NewDSSet(ctx, luciProject).Add(ctx, []dsset.Item{{
		ID:    uuid.New().String(),
		Value: value,
	}})
	if err != nil {
		return errors.Annotate(err, "failed to send event").Err()
	}
	return internal.Dispatch(ctx, luciProject, time.Time{} /*asap*/)
}
