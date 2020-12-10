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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/dsset"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/internal"
)

func init() {
	internal.PokeRunTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokeRunTask)
			switch err := pokeRunTask(ctx, run.ID(task.GetRunId())); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				return tq.Fatal.Apply(err)
			default:
				// TODO(tandrii/yiwzhang): avoid retries iff we know a new task was
				// already scheduled for the next second.
				return err
			}
		},
	)
}

const maxEventsToProcess = 256

func pokeRunTask(ctx context.Context, runID run.ID) error {
	mbox := internal.NewDSSet(ctx, string(runID))
	listing, err := mbox.List(ctx)
	if err != nil {
		return err
	}
	if err = dsset.CleanupGarbage(ctx, listing.Garbage); err != nil {
		return err
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		op, err := mbox.BeginPop(ctx, listing)
		if err != nil {
			return err
		}
		for _, mail := range listing.Items {
			if op.Pop(mail.ID) {
				e := internal.Event{}
				if err := proto.Unmarshal(mail.Value, &e); err != nil {
					return errors.Annotate(err, "failed to unmarshal event").Err()
				}
				logging.Debugf(ctx, "read %T", e.GetEvent())
			}
		}
		return dsset.FinishPop(ctx, op)
	}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to run %q", runID).Err()
	}
	return err
}
