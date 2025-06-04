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

package aggrmetrics

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/prjmanager"
)

var (
	metricPMStateCLs = metric.NewInt(
		"cv/internal/pm/cls",
		"Number of tracked CLs(# of vertices in the CL graph).",
		nil,
		field.String("project"),
	)
	metricPMStateDeps = metric.NewInt(
		"cv/internal/pm/deps",
		"Number of tracked CL dependencies (# of edges in the CL graph)",
		nil,
		field.String("project"),
	)
	metricPMStateComponents = metric.NewInt(
		"cv/internal/pm/components",
		"Number of tracked CL components (~ connected components in the CL graph)",
		nil,
		field.String("project"),
	)
	metricPMEntitySize = metric.NewInt(
		"cv/internal/pm/entity_size",
		"Approximate size of the Project entity in Datastore",
		&types.MetricMetadata{Units: types.Bytes},
		field.String("project"),
	)
)

// maxProjects is the max number of projects to which the pmReporter will
// scale in its current implementation.
//
// If a bigger amount is desired, batching should be implemented to avoid
// high memory usage.
const maxProjects = 500

type pmReporter struct{}

// metrics implements aggregator interface.
func (*pmReporter) metrics() []types.Metric {
	return []types.Metric{
		metricPMStateCLs,
		metricPMStateDeps,
		metricPMStateComponents,
		metricPMEntitySize,
	}
}

// report implements aggregator interface.
func (*pmReporter) report(ctx context.Context, projects []string) error {
	if len(projects) > maxProjects {
		logging.Errorf(ctx, "FIXME: too many active projects (>%d) to report metrics for", maxProjects)
		return errors.New("too many active projects")
	}

	// We need to load & decode Datastore entities as prjmanager.Project objects
	// in order to compute metrics like the number of components.
	// But we also need to figure out the Datastore entities' size in bytes, which
	// is unfortunately not easy to compute off the prjmanager.Project objects.
	//
	// So, load each project entity as both prjmanager.Project and the
	// projectEntitySizeEstimator objects. Note, that both objects have
	// datastore's Kind is exactly the same (see `gae:"$kind..."` attribute in
	// both structs.
	//
	// Finally, behind the scenes, datastore library should optimize this to load
	// only N entities instead of 2*N.
	eSizes := make([]projectEntitySizeEstimator, len(projects))
	entities := make([]prjmanager.Project, len(projects))
	i := 0
	for _, p := range projects {
		eSizes[i].ID = p
		entities[i].ID = p
		i++
	}
	err := datastore.Get(ctx, eSizes, entities)
	if err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to fetch all projects: %w", err))
	}

	for i, e := range entities {
		project := eSizes[i].ID
		metricPMEntitySize.Set(ctx, eSizes[i].bytes, project)
		metricPMStateCLs.Set(ctx, int64(len(e.State.GetPcls())), project)
		metricPMStateComponents.Set(ctx, int64(len(e.State.GetComponents())), project)
		deps := 0
		for _, pcl := range e.State.GetPcls() {
			deps += len(pcl.GetDeps())
		}
		metricPMStateDeps.Set(ctx, int64(deps), project)
	}
	return nil
}

type projectEntitySizeEstimator struct {
	// Kind must match the kind of the prjmanager.Project entity.
	_kind string `gae:"$kind,Project"`
	ID    string `gae:"$id"`
	bytes int64
}

// Load implements datastore.PropertyLoadSaver.
func (e *projectEntitySizeEstimator) Load(m datastore.PropertyMap) error {
	e.bytes = m.EstimateSize()
	return nil
}

// Save implements datastore.PropertyLoadSaver.
func (e *projectEntitySizeEstimator) Save(withMeta bool) (datastore.PropertyMap, error) {
	panic("not needed")
}
