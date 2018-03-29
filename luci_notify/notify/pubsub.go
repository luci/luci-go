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

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/proto"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/luci_notify/config"
)

var (
	errBuilderDeleted = fmt.Errorf("builder deleted between datastore.Get calls")
)

func getCommitBuildSet(sets []buildbucketpb.BuildSet) *buildbucketpb.GitilesCommit {
	for _, set := range sets {
		if commit, ok := set.(*buildbucketpb.GitilesCommit); ok {
			return commit
		}
	}
	return nil
}

func getBuilderID(b *Build) string {
	return fmt.Sprintf("buildbucket/%s/%s/%s", b.Project, b.Bucket, b.Builder)
}

func commitIndex(commits []*gitpb.Commit, revision string) int {
	for i, commit := range commits {
		if commit.Id == revision {
			return i
		}
	}
	return -1
}

func extractEmailNotifyValues(parametersJSON string) ([]string, error) {
	if parametersJSON == "" {
		return nil, nil
	}

	// json equivalent: {"email_notify": [{"email": "<address>"}, ...]}
	var output struct {
		EmailNotify []struct {
			Email string `json:"email"`
		} `json:"email_notify"`
	}

	if err := json.NewDecoder(strings.NewReader(parametersJSON)).Decode(&output); err != nil {
		return nil, errors.Annotate(err, "invalid msg.ParametersJson").Err()
	}

	result := make([]string, len(output.EmailNotify))
	for i, r := range output.EmailNotify {
		result[i] = r.Email
	}
	return result, nil
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
	builderID := getBuilderID(build)
	logging.Infof(c, "Finding config for %q, %s", builderID, build.Status)
	notifiers, err := config.LookupNotifiers(c, build.Project, builderID)
	if err != nil {
		return errors.Annotate(err, "looking up notifiers").Err()
	}
	if len(notifiers) == 0 {
		// No configurations were found, but we might need to generate notifications
		// based on properties. Don't fall through to avoid storing build status for
		// builds without configurations.
		return Notify(c, d, notifiers, StatusUnknown, build)
	}

	gCommit := getCommitBuildSet(build.BuildSets)
	if gCommit == nil {
		logging.Infof(c, "No revision information found for this build, ignoring...")
		return nil
	}

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
		switch {
		// If the revision is not found, we can conclude that the Builder has
		// advanced beyond gCommit.Revision. This is because:
		//   1) builder.StatusRevision only ever moves forward.
		//   2) commits contains the git history up to gCommit.Revision.
		case index < 0:
			logging.Debugf(c, "Found build with old commit during transaction.")
			outOfOrder = true

		// If the revision is current, check build creation time.
		case index == 0 && builder.StatusBuildTime.After(build.CreationTime):
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

// BuildbucketPubSubHandler is the main entrypoint for a new update from buildbucket's pubsub.
//
// This handler delegates the actual processing of the build to handleBuild.
// Its primary purpose is to unwrap context boilerplate and deal with progress-stopping errors.
func BuildbucketPubSubHandler(ctx *router.Context, d *tq.Dispatcher) {
	c, h := ctx.Context, ctx.Writer
	build, err := extractBuild(c, ctx.Request)
	switch {
	case err != nil:
		logging.WithError(err).Errorf(c, "error while extracting build")
	case !strings.HasPrefix(build.Bucket, "luci."):
		logging.Infof(c, "Received build that isn't part of LUCI, ignoring...")
	case !build.Status.Completed():
		logging.Infof(c, "Received build that hasn't completed yet, ignoring...")
	default:
		if err := handleBuild(c, d, build, gitilesHistory); err != nil {
			logging.WithError(err).Errorf(c, "error while notifying")
			if transient.Tag.In(err) {
				// Transient errors are 500 so that PubSub retries them.
				h.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
	h.WriteHeader(http.StatusOK)
}

// Build is buildbucket.Build along with the parsed 'email_notify' values.
type Build struct {
	buildbucket.Build
	EmailNotify []string
}

// extractBuild constructs a Build from the PubSub HTTP request.
func extractBuild(c context.Context, r *http.Request) (*Build, error) {
	// sent by pubsub.
	// This struct is just convenient for unwrapping the json message
	var msg struct {
		Message struct {
			Data []byte
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		return nil, errors.Annotate(err, "could not decode message").Err()
	}

	var message struct {
		Build bbapi.ApiCommonBuildMessage
	}
	if err := json.Unmarshal(msg.Message.Data, &message); err != nil {
		return nil, errors.Annotate(err, "could not parse pubsub message data").Err()
	}
	var build Build
	if err := build.ParseMessage(&message.Build); err != nil {
		return nil, errors.Annotate(err, "could not decode buildbucket build").Err()
	}

	emails, err := extractEmailNotifyValues(message.Build.ParametersJson)
	if err != nil {
		return nil, errors.Annotate(err, "could not decode email_notify").Err()
	}
	build.EmailNotify = emails

	return &build, nil
}
