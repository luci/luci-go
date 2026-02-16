// Copyright 2026 The LUCI Authors.
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

package resultingester

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/tracing"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

type RootInvocationInputs struct {
	Sources      *analysispb.Sources
	Notification *rdbpb.TestResultsNotification
}

// HandlePubSub is the entry point for ingesting data from a root invocation.
func (o *Orchestrator) HandlePubSub(ctx context.Context, n *rdbpb.TestResultsNotification) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.Orchestrator.HandlePubSub")
	defer func() { tracing.End(s, err) }()

	// For reading the root invocation, use a ResultDB client acting as
	// the project of the root invocation.
	project, _ := realms.Split(n.RootInvocationMetadata.Realm)

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		// Project not enabled for data ingestion.
		return nil
	}

	// The data is going into the project of the root invocation, so we should
	// query with the authority of that project.
	sources, err := gerritchangelists.PopulateOwnerKinds(ctx, project, n.RootInvocationMetadata.Sources)
	if err != nil {
		return errors.Fmt("populate changelist owner kinds: %w", err)
	}

	inputs := RootInvocationInputs{
		Sources:      sources,
		Notification: n,
	}

	for _, sink := range o.sinks {
		if err := sink.IngestRootInvocation(ctx, inputs); err != nil {
			return transient.Tag.Apply(errors.Fmt("ingest: %q: %w", sink.Name(), err))
		}
	}
	return nil
}
