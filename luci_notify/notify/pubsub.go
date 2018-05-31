// Copyright 2017 The LUCI Authors.
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

package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/tq"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/luci_notify/config"
)

var (
	// buildFieldMask defines which buildbucketpb.Build fields to fetch and
	// make available to email templates.
	// If a message field is specified here without periods, e.g. "steps", all
	// of its subfields are included too.
	buildFieldMask = &field_mask.FieldMask{
		Paths: []string{
			"id",
			"builder",
			"number",
			"created_by",
			"view_url",
			"create_time",
			"start_time",
			"end_time",
			"update_time",
			"status",
			"input",
			"output",
			"steps",
			"infra",
			"tags",
		},
	}
)

func getBuilderID(b *buildbucketpb.Build) string {
	return fmt.Sprintf("%s/%s", b.Builder.Bucket, b.Builder.Builder)
}

// handleBuild processes a Build and sends appropriate notifications.
//
// This function should serve as documentation of the process of going from
// a Build to sent notifications. It also should explicitly handle ACLs and
// stop the process of handling notifications early to avoid wasting compute time.
//
// history is a function that contacts gitiles to obtain the git history for
// revision ordering purposes. It's passed in as a parameter in order to mock it
// for testing.
func handleBuild(c context.Context, d *tq.Dispatcher, build *buildbucketpb.Build, history HistoryFunc) error {
	project := &config.Project{Name: build.Builder.Project}
	switch ex, err := datastore.Exists(c, project); {
	case err != nil:
		return err
	case !ex.All():
		return nil // This project is not tracked by luci-notify
	}

	notifier := Notifier{Build: build}
	gCommit := build.Input.GetGitilesCommit()
	buildCreateTime, _ := ptypes.Timestamp(build.CreateTime)

	checkout, err := getCheckout(c, build)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve source manifest").Err()
	}

	// Get the Builder for the first time, and initialize if there's nothing there.
	builderID := getBuilderID(build)
	builder := config.Builder{
		ProjectKey: datastore.KeyForObj(c, project),
		ID:         builderID,
	}
	var updatedBuilder config.Builder
	keepGoing := false
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, &builder); {
		case err == datastore.ErrNoSuchEntity:
			// Even if the builder isn't found, we may still want to notify if the build
			// specifies email addresses to notify.
			logging.Infof(c, "No builder %q found for project %q", builderID, build.Builder.Project)
			return notifier.Notify(c, d)
		case err != nil:
			return errors.Annotate(err, "failed to get builder").Tag(transient.Tag).Err()
		}

		// Update the notifier with the builder's notifications.
		notifier.Notifications = builder.Notifications

		// Create a new builder as a copy of the old, updated with build information.
		updatedBuilder = builder
		updatedBuilder.Status = build.Status
		updatedBuilder.StatusBuildTime = buildCreateTime
		updatedBuilder.StatusGitilesCommits = checkout.ToGitilesCommits()

		var recipients []EmailNotify
		switch {
		case builder.Repository == "":
			// Handle the case where there's no repository being tracked.
			if builder.StatusBuildTime.After(buildCreateTime) {
				logging.Infof(c, "Found build with old time")
				return notifier.Notify(c, d)
			}
			// Explicitly set OldStatus to enable on_change notifications.
			notifier.OldStatus = builder.Status
			if err := notifier.Notify(c, d); err != nil {
				return err
			}
			return datastore.Put(c, &updatedBuilder)
		case gCommit == nil:
			// If there's no revision information, and the builder has a repository, ignore
			// the build.
			logging.Infof(c, "No revision information found for this build, ignoring...")
			return nil
		}

		// Update the new builder with revision information as we know it's now available.
		updatedBuilder.StatusRevision = gCommit.Id

		// If there's no revision information on the Builder, this means the Builder
		// is uninitialized. Notify about the build as best as we can and then store
		// the updated builder.
		if builder.StatusRevision == "" {
			if err := notifier.Notify(c, d); err != nil {
				return err
			}
			return datastore.Put(c, &updatedBuilder)
		}
		keepGoing = true
		return nil
	}, nil)
	if err != nil || !keepGoing {
		return err
	}

	builderRepoHost, builderRepoProject, _ := gitiles.ParseRepoURL(builderRepository)
	if builderRepoHost != gCommit.Host || builderRepoProject != gCommit.Project {
		logging.Infof(c, "Builder %s triggered by commit to https://%s/%s"+
			"instead of known https://%s, ignoring...",
			builderID, gCommit.Host, gCommit.Project, builderRepository)
		return nil
	}

	// Get the revision history for the build-related commit.
	commits, err := history(c, gCommit.Host, gCommit.Project, builder.StatusRevision, gCommit.Id)
	if len(commits) == 0 {
		logging.Debugf(c, "Found build with old commit, ignoring...")
		return notifier.Notify(c, d)
	}

	// Get the blamelist logs, if needed.
	var blamelistLogs Logs
	if builder.BlamelistNotification != nil {
		whitelist := builder.BlamelistNotification.GetRepositoryWhitelist()

		// If the whitelist is non-empty, use the GitilesCommits to compute
		// the blamelist logs. Otherwise, just use the commits for the builder's
		// repository.
		if len(whitelist) != 0 {
			oldCheckout := NewCheckout(builder.StatusGitilesCommits)
			blamelistLogs, err = ComputeLogs(c, oldCheckout, checkout.Filter(whitelist), history) 
			if err != nil {
				logging.WithError(err).Warningf(c, "Failed to populate manifest history, ignoring blamelist...")
			}
		} else {
			blamelistLogs = make(Logs)
			blamelistLogs[builder.Repository] = commits
		}
	}

	// Update `builder`, and check if we need to store a newer version, then store it.
	oldRepository := builder.Repository
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, &builder); {
		case err == datastore.ErrNoSuchEntity:
			return errors.New("builder deleted between datastore.Get calls")
		case err != nil:
			return err
		}

		// If the builder's repository got updated in the meanwhile, we need to throw a
		// transient error and retry this whole thing.
		if builder.Repository != oldRepository {
			return errors.Reason("failed to notify because builder repository updated").Tag(transient.Tag).Err()
		}

		// Update the updatedBuilder with any new information we could have gotten from this
		// second datastore Get. Note that we know for sure that the project key, ID, and
		// repository are unchanged. We don't care about the Status* fields since that's what
		// we're updating.
		updatedBuilder.Notifications = builder.Notifications
		updatedBuilder.BlamelistNotification = builder.BlamelistNotification

		// Update the notifier's notifications too, since that may have changed.
		notifier.Notifications = builder.Notifications

		index := commitIndex(commits, builder.StatusRevision)
		outOfOrder := false
		switch {
		// If the revision is not found, we can conclude that the Builder has
		// advanced beyond gCommit.Revision. This is because:
		//   1) builder.StatusRevision only ever moves forward.
		//   2) commits contains the git history up to gCommit.Revision.
		case index < 0:
			logging.Debugf(c, "Found build with old commit during transaction.")
			outOfOrder = true

		// If the revision is current, check build creation time.
		case index == 0 && builder.StatusBuildTime.After(buildCreateTime):
			logging.Debugf(c, "Found build with the same commit but an old time.")
			outOfOrder = true
		}

		if outOfOrder {
			// If the build is out-of-order, we want to ignore only on_change notifications.
			return notifier.Notify(c, d)
		}
		notifier.OldStatus = builder.Status

		// If we're notifying the blamelist, trim the blamelist if necessary given the new
		// updated builder information, compute a blamelist from the logs, and add it to
		// the list of recipients.
		if builder.BlamelistNotification != nil {
			if len(blamelistNotification.GetRepositoryWhitelist()) != 0 {
				blamelistLogs = blamelistLogs.Trim(NewCheckout(builder.StatusGitilesCommits))
			}
			notifier.Blamelist = blamelistLogs.Blamelist(builder.BlamelistNotification.Template)
		}

		// Notify the final, complete list of recipients, and then update the builder.
		if err := notifier.Notify(c, d); err != nil {
			return err
		}
		return datastore.Put(c, &updatedBuilder)
	}, nil)
	return errors.Annotate(err, "failed to save builder").Tag(transient.Tag).Err()
}

func newBuildsClient(c context.Context, host string) (buildbucketpb.BuildsClient, error) {
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return buildbucketpb.NewBuildsPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: t},
		Host: host,
	}), nil
}

// BuildbucketPubSubHandler is the main entrypoint for a new update from buildbucket's pubsub.
//
// This handler delegates the actual processing of the build to handleBuild.
// Its primary purpose is to unwrap context boilerplate and deal with progress-stopping errors.
func BuildbucketPubSubHandler(ctx *router.Context, d *tq.Dispatcher) error {
	c := ctx.Context
	build, err := extractBuild(c, ctx.Request)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to extract build").Err()

	case build == nil:
		// Ignore.
		return nil

	default:
		return handleBuild(c, d, build, gitilesHistory)
	}
}

// extractBuild constructs a Build from the PubSub HTTP request.
func extractBuild(c context.Context, r *http.Request) (*buildbucketpb.Build, error) {
	// sent by pubsub.
	// This struct is just convenient for unwrapping the json message
	var msg struct {
		Message struct {
			Data []byte
		}
		Attributes map[string]interface{}
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		return nil, errors.Annotate(err, "could not decode message").Err()
	}

	if v, ok := msg.Attributes["version"].(string); ok && v != "v1" {
		// Ignore v2 pubsub messages. TODO(nodir): use v2.
		return nil, nil
	}
	var message struct {
		Build    bbv1.ApiCommonBuildMessage
		Hostname string
	}
	switch err := json.Unmarshal(msg.Message.Data, &message); {
	case err != nil:
		return nil, errors.Annotate(err, "could not parse pubsub message data").Err()
	case !strings.HasPrefix(message.Build.Bucket, "luci."):
		logging.Infof(c, "Received build that isn't part of LUCI, ignoring...")
		return nil, nil
	case message.Build.Status != bbv1.StatusCompleted:
		logging.Infof(c, "Received build that hasn't completed yet, ignoring...")
		return nil, nil
	}

	buildsClient, err := newBuildsClient(c, message.Hostname)
	if err != nil {
		return nil, err
	}

	logging.Infof(c, "fetching build %d", message.Build.Id)
	res, err := buildsClient.GetBuild(c, &buildbucketpb.GetBuildRequest{
		Id:     message.Build.Id,
		Fields: buildFieldMask,
	})
	if status.Code(err) == codes.NotFound {
		logging.Warningf(c, "no access to build %d", message.Build.Id)
		return nil, nil
	}
	return res, errors.Annotate(err, "could not fetch buildbucket build %d", message.Build.Id).Err()
}
