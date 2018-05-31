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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	notifyConfig "go.chromium.org/luci/luci_notify/config"
)

var (
	errBuilderDeleted = fmt.Errorf("builder deleted between datastore.Get calls")

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

func getBuilderID(b *Build) string {
	return "buildbucket/" + b.Builder.IDString()
}

func commitIndex(commits []*gitpb.Commit, revision string) int {
	for i, commit := range commits {
		if commit.Id == revision {
			return i
		}
	}
	return -1
}

// EmailNotify contains information for delivery and personalization of notification emails.
type EmailNotify struct {
	Email    string `json:"email"`
	Template string `json:"template"`
}

func extractEmailNotifyValues(parametersJSON string) ([]EmailNotify, error) {
	if parametersJSON == "" {
		return nil, nil
	}
	// json equivalent: {"email_notify": [{"email": "<address>"}, ...]}
	var output struct {
		EmailNotify []EmailNotify `json:"email_notify"`
	}

	if err := json.NewDecoder(strings.NewReader(parametersJSON)).Decode(&output); err != nil {
		return nil, errors.Annotate(err, "invalid msg.ParametersJson").Err()
	}
	return output.EmailNotify, nil
}

// notifyFunc represents a closed-over version of Notify with the other arguments already curried.
type notifyFunc func(c context.Context, status buildbucketpb.Status) error

// lookupNotifier looks up the notifier for the build (related to the build's builder) in the datastore and returns it.
//
// Returns a list of notifiers, whether we should continue, and potentially an error.
func lookupNotifier(c context.Context, build *Build, notify notifyFunc) (*notifyConfig.Notifier, bool, error) {
	builderID := getBuilderID(build)
	logging.Infof(c, "Finding config for %q, %s", builderID, build.Status)
	notifier, err := notifyConfig.LookupNotifiers(c, build.Builder.Project, builderID)
	if err != nil {
		return nil, false, errors.Annotate(err, "looking up notifiers").Tag(transient.Tag).Err()
	}
	if notifier == nil {
		// No configurations were found, but we might need to generate notifications
		// based on properties. Don't fall through to avoid storing build status for
		// builds without configurations.
		return nil, false, notify(c, notifyConfig.StatusUnknown)
	}
	return notifier, true, nil
}

// tryHandleBuild attempts to handle a build without revision information available.
//
// Returns a gitiles commit (signifying that it's non-nil), the builder from the datastore, whether we
// should keep going, and potentially an error.
func tryHandleBuild(c context.Context, manifest *srcman.Manifest, build *Build, notify notifyFunc) (*buildbucketpb.GitilesCommit, *notifyConfig.Builder, bool, error) {
	gCommit := build.Input.GetGitilesCommit()

	// Get the Builder for the first time, and initialize if there's nothing there.
	builderID := getBuilderID(build)
	builder := &Builder{ID: builderID}
	keepGoing := false
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, builder); {
		case err == datastore.ErrNoSuchEntity:
			// If the Builder isn't found, this means there's an inconsistency in the datastore,
			// since Builders are managed by the configuration. It effectively means that there are
			// notifiers for a builder, but the builder itself is gone. This should never happen as
			// builders and notifiers are updated in transactions.
			return errors.New("notifier referred to non-existent builder %s", builderID)
		case err != nil:
			return errors.Annotate(err, "failed to get builder").Tag(transient.Tag).Err()
		}

		buildCreateTime, _ := ptypes.Timestamp(build.CreateTime)

		// Handle the case where there's no repository being tracked.
		if builder.Repository == "" {
			if builder.StatusBuildTime.Before(buildCreateTime) {
				if err := notify(c, builder.Status); err != nil {
					return err
				}
				builder.StatusBuildTime = buildCreateTime
				builder.StatusSourceManifest = manifest
				return datastore.Put(c, builder)
			}
			logging.Infof(c, "Found build with old time")
			return notify(c, notifyConfig.StatusUnknown)
		}
		// If there's no revision information, and the builder has a repository, ignore
		// the build.
		if gCommit == nil {
			logging.Infof(c, "No revision information found for this build, ignoring...")
			return nil
		}
		// Check if the repository matches 
		repoURL, _ := url.Parse(builder.Repository)
		if repoURL.Hostname() != gCommit.Host || repoURL.EscapedPath() != gCommit.Project {
			logging.Infof(c,
				"Builder %s triggered by commit to %s on %s instead of known repo %s, ignoring...",
				builderID, gCommit.Host, gCommit.Project, builder.Repository)
			return nil
		}
		// Handle the case where the builder hasn't seen a build yet.
		if builder.StatusRevision == "" {
			if err := notify(c, notifyConfig.StatusUnknown); err != nil {
				return err
			}
			builder.StatusBuildTime = buildCreateTime
			builder.StatusRevision = gCommit.Id
			builder.StatusSourceManifest = manifest
			return datastore.Put(c, builder)
		}
		keepGoing = true
	})
	return gCommit, builder, keepGoing, err
}

// getCommitHistory retrieves the commit history between the builder's revision and the build's revision.
// It also produces a diff between the builder's source manifest and the build's manifest, which is then
// populated with a git history.
//
// Returns a list of git commits, whether to keep going, and potentially an error.
func getCommitHistory(c context.Context, builder *notifyConfig.Builder, gCommit *buildbucketpb.GitilesCommit, manifest *srcman.Manifest, notify notifyFunc, history HistoryFunc) (*srcman.ManifestDiff, []*gitpb.Commit, bool, error) {
	var diff *srcman.ManifestDiff
	var commits []*gitpb.Commit
	err := parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			diff = builder.StatusSourceManifest.Diff(manifest)
			err := populateHistory(c, diff, history)
			if err != nil {
				logging.WithError(err).Warningf(c, "Failed to populate manifest history, ignoring blamelist...")
			}
			return nil
		}
		work <- func() error {
			var err error
			// commits contains a list of commits from builder.StatusRevision..gCommit.Revision (oldest..newest).
			commits, err = history(c, gCommit.Host, gCommit.Project, builder.StatusRevision, gCommit.Id)
			return err
		}
	})
	if err != nil {
		return nil, false, err
	}
	if len(commits) == 0 {
		logging.Debugf(c, "Found build with old commit, ignoring...")
		// Notify about the build, but ignore on_change by creating a builder with an unknown status.
		return nil, false, notify(c, StatusUnknown)
	}
	return commits, true, nil
}

// updateBuilderAndNotify uses revision information to determine if the build we're processing
// arrived out-of-order compared to the builder's previous build, updates the builder in the datastore
// if not, and sends a notification.
func updateBuilderAndNotify(c context.Context, commits []*gitpb.Commit, builder *notifyConfig.Builder, newRevision string, firstDiff *srcman.ManifestDiff, build *Build, notify notifyFunc) error {
	// Update `builder`, and check if we need to store a newer version, then store it.
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, builder); {
		case err == datastore.ErrNoSuchEntity:
			return errBuilderDeleted
		case err != nil:
			return err
		}

		diff := builder.StatusSourceManifest.Diff(firstDiff.Old)
		// Attempt to populate the diff with git history from the first diff.
		// This is only guaranteed to work if source manifests only ever move
		// forward, which may not always be true. In practice, it's generally true
		// however. At worst, commits not in the blamelist will be lost.
		populateHistoryFromDiff(diff, firstDiff)
		blamelist := blamelistFromDiff(diff)

		index := commitIndex(commits, builder.StatusRevision)
		buildCreateTime, _ := ptypes.Timestamp(build.CreateTime)
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

		// If the build is out-of-order, we want to ignore only on_change notifications.
		// Since on_change only occurs when Builder Status != StatusUnknown, set builder
		// to a new builder with exactly this property.
		if outOfOrder {
			return notify(c, StatusUnknown)
		}
		if err := notify(c, builder.Status); err != nil {
			return err
		}
		builder.StatusRevision = newRevision
		builder.StatusBuildTime = buildCreateTime
		builder.StatusSourceManifest = manifest
		return datastore.Put(c, builder)
	}, nil)
	return errors.Annotate(err, "failed to save builder").Tag(transient.Tag).Err()
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
func handleBuild(c context.Context, d *tq.Dispatcher, build *Build, history HistoryFunc) error {
	switch ex, err := datastore.Exists(c, &notifyConfig.Project{Name: build.Builder.Project}); {
	case err != nil:
		return err
	case !ex.All():
		return nil // This project is not tracked by luci-notify
	}

	var notifier *notifyConfig.Notifier
	notify := func(c context.Context, status buildbucketpb.Status) error {
		return Notify(c, d, notifier, status, build)
	}
	notifiers, ok, err := lookupNotifier(c, build, notify)
	if err != nil || !ok {
		return err
	}
	manifest, err := getSourceManifest(c, build)
	if err != nil {
		logging.Warningf(c, "No source manifest found for this build, ignoring blamelist...")
		manifest = nil
	}
	gCommit, builder, ok, err := tryHandleBuild(c, manifest, build, notify)
	if err != nil || !ok {
		return err
	}
	diff, commits, ok, err := getCommitHistory(c, builder, gCommit, manifest, notify, history)
	if err != nil || !ok {
		return err
	}
	return updateBuilderAndNotify(c, commits, builder, gCommit.Id, diff, build, notify)
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

// Build is buildbucketpb.Build along with the parsed 'email_notify' values.
type Build struct {
	buildbucketpb.Build
	EmailNotify []EmailNotify
}

// extractBuild constructs a Build from the PubSub HTTP request.
func extractBuild(c context.Context, r *http.Request) (*Build, error) {
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
	res, err := buildsClient.GetBuild(c, &buildbucketpb.GetBuildRequest{
		Id:     message.Build.Id,
		Fields: buildFieldMask,
	})
	switch {
	case status.Code(err) == codes.NotFound:
		logging.Warningf(c, "no access to build %d", message.Build.Id)
		return nil, nil
	case err != nil:
		return nil, errors.Annotate(err, "could not fetch buildbucket build %d", message.Build.Id).Err()
	}

	emails, err := extractEmailNotifyValues(message.Build.ParametersJson)
	if err != nil {
		return nil, errors.Annotate(err, "could not decode email_notify").Err()
	}

	return &Build{
		Build:       *res,
		EmailNotify: emails,
	}, nil
}
