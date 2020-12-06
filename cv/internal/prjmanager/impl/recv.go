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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/prjmanager"
)

func init() {
	prjmanager.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			return errors.Reason("TODO(tandrii): implement").Tag(tq.Fatal).Err()
		},
	)
}

// peek reads up to `limit` outstanding events in no particular order.
//
// Must not be called from a transaction because the events are immutable.
func peek(ctx context.Context, luciProject string, limit int) ([]*prjmanager.Event, error) {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("must be called outside a transaction")
	}
	return nil, errors.New("TODO(tandrii): implement")
}

// delete deletes events returned by Peek.
//
// Already deleted events are silently ignored.
//
// Should be called in a transaction with the necessary mutation which saves
// result of processing of the event.
//
// May be called outside a transaction iff the event has definitely been
// processed already.
func delete(ctx context.Context, luciProject string, events []*prjmanager.Event) error {
	return errors.New("TODO(tandrii): implement")
}
