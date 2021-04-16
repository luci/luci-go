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

package eventpb

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
)

// SendNow sends the event to Run's eventbox and invokes RunManager immediately.
func SendNow(ctx context.Context, runID common.RunID, evt *Event) error {
	return Send(ctx, runID, evt, time.Time{})
}

// Send sends the event to Run's eventbox and invokes RunManager at `eta`.
func Send(ctx context.Context, runID common.RunID, evt *Event, eta time.Time) error {
	if err := SendWithoutDispatch(ctx, runID, evt); err != nil {
		return err
	}
	return Dispatch(ctx, string(runID), eta)
}

// SendWithoutDispatch sends the event to Run's eventbox without invoking RM.
func SendWithoutDispatch(ctx context.Context, runID common.RunID, evt *Event) error {
	value, err := proto.Marshal(evt)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	return eventbox.Emit(ctx, value, datastore.MakeKey(ctx, "Run", string(runID)))
}
