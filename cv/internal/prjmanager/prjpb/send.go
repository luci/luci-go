// Copyright 2021 The LUCI Authors.
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

package prjpb

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/eventbox"
)

// SendNow sends the event to Project's eventbox and invokes PM immediately.
func SendNow(ctx context.Context, luciProject string, e *Event) error {
	if err := SendWithoutDispatch(ctx, luciProject, e); err != nil {
		return err
	}
	return Dispatch(ctx, luciProject, time.Time{} /*asap*/)
}

// Send sends the event to Project's eventbox without invoking a PM.
func SendWithoutDispatch(ctx context.Context, luciProject string, e *Event) error {
	value, err := proto.Marshal(e)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	// Must be same as prjmanager.ProjectKind, but can't import due to circular
	// imports.
	to := datastore.MakeKey(ctx, "Project", luciProject)
	return eventbox.Emit(ctx, value, to)
}
