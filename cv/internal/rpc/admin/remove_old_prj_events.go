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

package admin

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox/dsset"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

var removeOldProjectEventsConfig = dsmapper.JobConfig{
	Mapper: "project-event-cleanup",
	Query: dsmapper.Query{
		// We iterate over all Projects, and then read and cleanup their events.
		Kind: "Project",
	},
	PageSize:   1,
	ShardCount: 4,
}

var ignoreProjectsAfter = time.Date(2021, time.October, 1, 0, 0, 0, 0, time.UTC)

var removeOldProjectEventsFactory = func(ctx context.Context, j *dsmapper.Job, shard int) (dsmapper.Mapper, error) {
	tsJobName := string(j.Config.Mapper)
	tsJobID := int64(j.ID)

	upgradeProject := func(ctx context.Context, pr *prjmanager.Project) error {
		if pr.UpdateTime.After(ignoreProjectsAfter) {
			return nil
		}

		set := dsset.Set{
			Parent:          datastore.MakeKey(ctx, prjmanager.ProjectKind, pr.ID),
			TombstonesDelay: time.Second,
		}
		const maxEvents = 10000
		listing, err := set.List(ctx, 10000)
		switch {
		case err != nil:
			return err
		case len(listing.Items) == 0:
			return nil
		case len(listing.Items) == maxEvents:
			// Return a permanent error to fail the shard.
			return fmt.Errorf("project %q has too many outstanding events >= %d", pr.ID, maxEvents)
		}

		// Cleanup already processed Project events.
		if err := dsset.CleanupGarbage(ctx, listing.Garbage); err != nil {
			return err
		}

		// Identify any old events for deletion.
		var toDelete []string
		for _, item := range listing.Items {
			evt := &prjpb.Event{}
			if err = proto.Unmarshal(item.Value, evt); err != nil {
				return errors.Annotate(err, "project %q event %q failed to decode event", pr.ID, item.ID).Err()
			}
			if evt.GetClUpdated() != nil {
				toDelete = append(toDelete, item.ID)
				continue
			}
		}
		if len(toDelete) == 0 {
			return nil
		}

		var popped int64
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			popped = 0
			op, err := set.BeginPop(ctx, listing)
			if err != nil {
				return err
			}
			for _, d := range toDelete {
				if op.Pop(d) {
					popped++
				}
			}
			return dsset.FinishPop(ctx, op)
		}, nil)
		if err != nil {
			return errors.Annotate(err, "failed to delete events for project %q", pr.ID).Tag(transient.Tag).Err()
		}
		metricUpgraded.Add(ctx, int64(popped), tsJobName, tsJobID, "Project: event")
		return nil
	}

	upgradeManyProjects := func(ctx context.Context, keys []*datastore.Key) error {
		errs := parallel.FanOutIn(func(work chan<- func() error) {
			for _, key := range keys {
				key := key
				work <- func() error {
					pr, err := prjmanager.Load(ctx, key.StringID())
					if err != nil {
						return err
					}
					return upgradeProject(ctx, pr)
				}
			}
		})
		return common.MostSevereError(errs)
	}

	return upgradeManyProjects, nil
}
