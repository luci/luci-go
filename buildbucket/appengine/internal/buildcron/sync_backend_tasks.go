// Copyright 2023 The LUCI Authors.
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

package buildcron

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
)

type projectBackendPair struct {
	// LUCI project name.
	project string
	// Task backend target.
	backend string
}

// Find distinct (project, backend_target) by querying Build.
// TODO(crbug.com/1472896): cache the (project, backend) pairs.
func getProjectBackendPairs(ctx context.Context) ([]*projectBackendPair, error) {
	q := datastore.NewQuery(model.BuildKind).
		Eq("incomplete", true).Gt("backend_target", "").
		Project("project", "backend_target").Distinct(true)
	var blds []*model.Build
	if err := datastore.GetAll(ctx, q, &blds); err != nil {
		return nil, errors.Fmt("failed to fetch all project keys: %w", err)
	}

	var pairs []*projectBackendPair
	for _, b := range blds {
		pairs = append(pairs, &projectBackendPair{project: b.Project, backend: b.BackendTarget})
	}
	return pairs, nil
}

// TriggerSyncBackendTasks finds all (project, backend_target) pairs and triggers
// SyncBuildsWithBackendTasks tasks for each pair.
func TriggerSyncBackendTasks(ctx context.Context) error {
	pairs, err := getProjectBackendPairs(ctx)
	if err != nil {
		return err
	}

	var merr errors.MultiError
	for _, p := range pairs {
		if err := tasks.SyncWithBackend(ctx, p.backend, p.project); err != nil {
			merr = append(merr, errors.Fmt("failed to enqueue task to sync builds of project %s with backend %s: %w", p.project, p.backend, err))
		}
	}
	return merr.AsError()
}
