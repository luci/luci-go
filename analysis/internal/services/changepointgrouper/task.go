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

// Package changepointgrouper defines the top-level task queue which groups
// changepoints from changepoint analysis and export it BigQuery table.
package changepointgrouper

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/groupexporter"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	groupChangepointsTaskClass = "group-changepoints"
	groupChangepointsQueue     = "group-changepoints"
)

var groupChangepoints = tq.RegisterTaskClass(tq.TaskClass{
	ID:        groupChangepointsTaskClass,
	Prototype: &taskspb.GroupChangepoints{},
	Queue:     groupChangepointsQueue,
	Kind:      tq.NonTransactional,
})

// RegisterTaskHandler registers the handler for group changepoints tasks.
func RegisterTaskHandler(srv *server.Server) error {
	changepointClient, err := changepoints.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Fmt("create changepoint BigQuery client: %w", err)
	}
	insertClient, err := groupexporter.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Fmt("create insert client: %w", err)
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := changepointClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up changepoint BigQuery client: %s", err)
		}
		err = insertClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up insert client: %s", err)
		}
	})
	grouper := &changepointGrouper{
		changepointClient: changepointClient,
		exporter:          *groupexporter.NewExporter(insertClient),
	}
	groupChangepoints.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.GroupChangepoints)
		return grouper.run(ctx, task)
	})
	return nil
}

// Schedule enqueues a task to group and export changepoint for a certain week.
func Schedule(ctx context.Context, week time.Time) error {
	return tq.AddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("week-%s", week.Format(time.RFC3339)),
		Payload: &taskspb.GroupChangepoints{
			Week:         timestamppb.New(week),
			ScheduleTime: timestamppb.New(clock.Now(ctx)),
		},
	})
}

type ChangePointClient interface {
	ReadChangepointsRealtime(ctx context.Context, week time.Time) ([]*changepoints.ChangepointDetailRow, error)
}

type changepointGrouper struct {
	changepointClient ChangePointClient
	exporter          groupexporter.Exporter
}

func (c *changepointGrouper) run(ctx context.Context, payload *taskspb.GroupChangepoints) (retErr error) {
	defer func() {
		if retErr != nil {
			// Add a fatal tag to all errors, so that it can show up in the tasks permanently failed SLO.
			// TODO (@beining): remove the fatal tag and add a freshness SLO to the grouped_changepoints table.
			retErr = tq.Fatal.Apply(retErr)
		}
	}()
	if payload.ScheduleTime.AsTime().Before(clock.Now(ctx).Add(-10 * time.Minute)) {
		return errors.New(`drop task older than 10 minutes to prevent over-growing the queue depth,
		it means grouped changepoint rows for this week is stale.`)
	}
	rows, err := c.changepointClient.ReadChangepointsRealtime(ctx, payload.Week.AsTime())
	if err != nil {
		return errors.Fmt("read BigQuery changepoints: %w", err)
	}
	changepointsByProject := splitByProject(rows)
	groups := [][]*changepoints.ChangepointDetailRow{}
	for _, project := range sortedKeys(changepointsByProject) {
		cps := changepointsByProject[project]
		groups = append(groups, changepoints.GroupChangepoints(ctx, cps)...)
	}
	now := clock.Now(ctx)
	if err := c.exporter.Export(ctx, groups, now); err != nil {
		return errors.Fmt("export groups to BigQuery: %w", err)
	}
	return nil
}

func splitByProject(cps []*changepoints.ChangepointDetailRow) map[string][]*changepoints.ChangepointDetailRow {
	splitted := map[string][]*changepoints.ChangepointDetailRow{}
	for _, cp := range cps {
		if _, ok := splitted[cp.Project]; !ok {
			splitted[cp.Project] = make([]*changepoints.ChangepointDetailRow, 0)
		}
		splitted[cp.Project] = append(splitted[cp.Project], cp)
	}
	return splitted
}

func sortedKeys(m map[string][]*changepoints.ChangepointDetailRow) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}
