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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/commitingester/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/pubsubutil"
)

// RegisterPubSubHandlers registers the pubsub handlers for Source Index's
// commit ingestion.
func RegisterPubSubHandlers(srv *server.Server) error {
	pusherID := identity.Identity(fmt.Sprintf("user:gitiles-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))

	// A middleware that restrict the endpoint to the pubsub service account.
	pubsubMW := router.NewMiddlewareChain(
		auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
			AudienceCheck: openid.AudienceMatchesHost,
		}),
		func(ctx *router.Context, next router.Handler) {
			if got := auth.CurrentIdentity(ctx.Request.Context()); got != pusherID {
				logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", pusherID, got)
				ctx.Writer.WriteHeader(http.StatusForbidden)
				return
			}
			next(ctx)
		},
	)

	// PubSub subscription endpoints.
	srv.Routes.POST("/push-handlers/gitiles/:gitiles_host", pubsubMW, pubSubHandler)

	return nil
}

func pubSubHandler(c *router.Context) {
	ctx := c.Request.Context()

	gitilesHost := c.Params.ByName("gitiles_host")

	// Parse event.
	var event gerritpb.SourceRepoEvent
	if err := json.NewDecoder(c.Request.Body).Decode(&event); err != nil {
		errors.Log(ctx, errors.Annotate(err, "could not decode gitiles pubsub message").Err())
		pubsubutil.HandleErr(c, err)
		return
	}

	// Process event.
	if err := processSourceRepoEvent(ctx, gitilesHost, &event); err != nil {
		errors.Log(ctx, errors.Annotate(err, "process event for host: %q", gitilesHost).Err())
		pubsubutil.HandleErr(c, err)
		return
	}

	c.Writer.WriteHeader(http.StatusAccepted)
}

// processSourceRepoEvent checks whether the given event should trigger a commit
// ingestion. If yes, schedule a commit-ingestion task.
func processSourceRepoEvent(ctx context.Context, gitilesHost string, event *gerritpb.SourceRepoEvent) error {
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "get the config").Err()
	}

	// Reject hosts that are not configured so if we only created the PubSub
	// subscription but forgot to configure the host, we will get an alert
	// (through SLO monitoring).
	if !cfg.HasHost(gitilesHost) {
		return errors.Reason("host %q is not configured to be indexed", gitilesHost).Err()
	}

	chunks := strings.SplitN(event.Name, "/", 4)
	if len(chunks) != 4 {
		logging.Errorf(ctx, "invalid name format; host: %q; name: %q;", gitilesHost, event.Name)
		return errors.New("invalid repository format")
	}
	repo := chunks[3]

	updateEvent := event.GetRefUpdateEvent()
	if updateEvent == nil {
		return nil
	}

	for _, update := range updateEvent.RefUpdates {
		if update.UpdateType == gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate_DELETE {
			continue
		}

		ref := update.RefName
		if !cfg.ShouldIndexRef(gitilesHost, repo, ref) {
			continue
		}

		f := func(ctx context.Context) error {
			scheduleCommitIngestion(ctx, &taskspb.IngestCommits{
				Host:       gitilesHost,
				Repository: repo,
				Commitish:  update.NewId,
				PageToken:  "",
				TaskIndex:  0,
			})
			return nil
		}
		if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
			if _, ok := status.FromError(errors.Unwrap(err)); ok {
				// Spanner gRPC error.
				return transient.Tag.Apply(err)
			}
			return err
		}
	}

	return nil
}