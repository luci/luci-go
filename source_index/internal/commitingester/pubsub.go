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

package commitingester

import (
	"context"
	"strings"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/commitingester/internal/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/pubsubutil"
)

var (
	repoEventCounter = metric.NewCounter(
		"source_index/ingestion/pubsub/gitiles",
		"The number of repo events received by LUCI Source Index from PubSub.",
		nil,
		// The Gitiles host.
		field.String("host"),
		// The Gitiles repository.
		field.String("repository"),
		// "success", "transient-failure" or "permanent-failure".
		field.String("status"))
)

// RegisterPubSubHandlers registers the pubsub handlers for Source Index's
// commit ingestion.
func RegisterPubSubHandlers(srv *server.Server) error {
	// Provide one handler for each message format. The message format is decided
	// by the host configuration. See
	// https://g3doc.corp.google.com/company/teams/gerritcodereview/users/cloud-pubsub.md#quickstart
	pubsub.RegisterJSONPBHandler("gitiles-jsonpb", pubSubHandler)
	pubsub.RegisterWirePBHandler("gitiles-wirepb", pubSubHandler)
	return nil
}

func pubSubHandler(ctx context.Context, message pubsub.Message, event *gerritpb.SourceRepoEvent) (err error) {
	repo := "unknown"
	// Extract host from URL query parameters, e.g. from
	// /internal/pubsub/gitiles?host=myhost.googlesource.com
	gitilesHost := message.Query.Get("host")

	ctx = logging.SetField(ctx, "host", gitilesHost)

	defer func() {
		// Closure for late binding.
		repoEventCounter.Add(ctx, 1, gitilesHost, repo, pubsubutil.StatusString(err))
	}()

	// Process event.
	repo, err = processSourceRepoEvent(ctx, gitilesHost, event)
	if err != nil {
		return errors.Fmt("process event for host %q: %w", gitilesHost, err)
	}
	return nil
}

// processSourceRepoEvent checks whether the given event should trigger a commit
// ingestion. If yes, schedule a commit-ingestion task.
func processSourceRepoEvent(ctx context.Context, gitilesHost string, event *gerritpb.SourceRepoEvent) (repo string, err error) {
	repo = "unknown"

	cfg, err := config.Get(ctx)
	if err != nil {
		return repo, errors.Fmt("get the config: %w", err)
	}

	// Reject hosts that are not configured so if we only created the PubSub
	// subscription but forgot to configure the host, we will get an alert
	// (through SLO monitoring).
	if !cfg.HasHost(gitilesHost) {
		return repo, errors.New("host is not configured to be indexed")
	}

	chunks := strings.SplitN(event.Name, "/", 4)
	if len(chunks) != 4 {
		logging.Errorf(ctx, "invalid event name: %q", event.Name)
		return repo, errors.New("unable to extract repository name")
	}
	repo = chunks[3]

	ctx = logging.SetField(ctx, "repository", repo)

	updateEvent := event.GetRefUpdateEvent()
	if updateEvent == nil {
		logging.Infof(ctx, "not a ref update event; skipping")
		return repo, nil
	}

	for _, update := range updateEvent.RefUpdates {
		ref := update.RefName
		ctx = logging.SetField(ctx, "ref", ref)

		if update.UpdateType == gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate_DELETE {
			logging.Infof(ctx, "deletion; skipping this ref update")
			continue
		}

		if !cfg.ShouldIndexRef(gitilesHost, repo, ref) {
			logging.Infof(ctx, "not configured to be indexed; skipping this ref update")
			continue
		}

		f := func(ctx context.Context) error {
			scheduleCommitIngestion(ctx, &taskspb.IngestCommits{
				Host:       gitilesHost,
				Repository: repo,
				Commitish:  update.NewId,
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   false,
			})
			return nil
		}
		if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
			if _, ok := status.FromError(errors.Unwrap(err)); ok {
				// Spanner gRPC error.
				return repo, transient.Tag.Apply(err)
			}
			return repo, err
		}
	}

	return repo, nil
}
