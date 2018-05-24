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

	builderID := getBuilderID(build)
	logging.Infof(c, "Finding config for %q, %s", builderID, build.Status)
	notifiers, err := notifyConfig.LookupNotifiers(c, build.Builder.Project, builderID)
	if err != nil {
		return errors.Annotate(err, "looking up notifiers").Err()
	}
	if len(notifiers) == 0 {
		// No configurations were found, but we might need to generate notifications
		// based on properties. Don't fall through to avoid storing build status for
		// builds without configurations.
		return Notify(c, d, notifiers, StatusUnknown, build)
	}

	if len(build.Input.GetGitilesCommits()) == 0 {
		logging.Infof(c, "No revision information found for this build, ignoring...")
		return nil
	}
	gCommit := build.Input.GitilesCommits[0]

	// Get the Builder for the first time, and initialize if there's nothing there.
	builder := &Builder{ID: builderID}
	switch err := datastore.Get(c, builder); {
	case err == datastore.ErrNoSuchEntity:
		// If the Builder isn't found, start a transaction and try to store
		// the builder for the first time.
		keepGoing := false
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			switch err := datastore.Get(c, builder); {
			case err == datastore.ErrNoSuchEntity:
				// Notify, but avoid on_change notification by setting Status to StatusUnknown.
				builder.Status = StatusUnknown
				if err := Notify(c, d, notifiers, builder.Status, build); err != nil {
					return err
				}
				// Initialize the Builder in the datastore.
				if err := datastore.Put(c, NewBuilder(builderID, gCommit.Id, &build.Build)); err != nil {
					return err
				}
				return nil
			case err != nil:
				return err
			}
			// If we actually managed to get a Builder, we should continue with the
			// full code path.
			keepGoing = true
			return nil
		}, nil)
		if err != nil || !keepGoing {
			return err
		}
	case err != nil:
		return err
	}

	// commits contains a list of commits from builder.StatusRevision..gCommit.Revision (oldest..newest).
	commits, err := history(c, gCommit.Host, gCommit.Project, builder.StatusRevision, gCommit.Id)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		logging.Debugf(c, "Found build with old commit, ignoring...")
		// Notify about the build, but ignore on_change by creating a builder with an unknown status.
		return Notify(c, d, notifiers, StatusUnknown, build)
	}

	// Update `builder`, and check if we need to store a newer version, then store it.
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, builder); {
		case err == datastore.ErrNoSuchEntity:
			return errBuilderDeleted
		case err != nil:
			return err
		}

		// If there's been no change in status, don't bother updating.
		if build.Status == builder.Status {
			return nil
		}

		index := commitIndex(commits, builder.StatusRevision)
		outOfOrder := false
		createTime, _ := ptypes.Timestamp(build.CreateTime)
		switch {
		// If the revision is not found, we can conclude that the Builder has
		// advanced beyond gCommit.Revision. This is because:
		//   1) builder.StatusRevision only ever moves forward.
		//   2) commits contains the git history up to gCommit.Revision.
		case index < 0:
			logging.Debugf(c, "Found build with old commit during transaction.")
			outOfOrder = true

		// If the revision is current, check build creation time.
		case index == 0 && builder.StatusBuildTime.After(createTime):
			logging.Debugf(c, "Found build with the same commit but an old time.")
			outOfOrder = true
		}

		// If the build is out-of-order, we want to ignore only on_change notifications.
		// Since on_change only occurs when Builder Status != StatusUnknown, set builder
		// to a new builder with exactly this property.
		if outOfOrder {
			return Notify(c, d, notifiers, StatusUnknown, build)
		}
		if err := Notify(c, d, notifiers, builder.Status, build); err != nil {
			return err
		}
		return datastore.Put(c, NewBuilder(builderID, gCommit.Id, &build.Build))
	}, nil)
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
		if err := handleBuild(c, d, build, gitilesHistory); err != nil {
			return errors.Annotate(err, "failed to notify").Tag(transient.Tag).Err()
		}
		return nil
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
		Id: message.Build.Id,
		Fields: &field_mask.FieldMask{
			Paths: []string{"steps"},
		},
	})
	if err != nil {
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
