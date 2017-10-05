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
	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

var (
	errAccessDenied = fmt.Errorf("access to build denied")
	errBuilderDeleted = fmt.Errorf("builder deleted between datastore.Get calls")
)

func getCommitBuildSet(sets []buildbucket.BuildSet) *buildbucket.GitilesCommit {
	for _, set := range sets {
		if commit, ok := set.(*buildbucket.GitilesCommit); ok {
			return commit
		}
	}
	return nil
}

// getCommitHistory gets a list of commits from Gitiles, the equivalent of the command
// `git log oldRevision..newRevision`.
func getCommitHistory(c context.Context, repoURL, newRevision, oldRevision string) ([]gitiles.Commit, error) {
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(
		"https://www.googleapis.com/auth/gerritcodereview",
	))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}

	// With this range, if newCommit.Revision == oldRevision we'll get nothing, but
	// this was checked earlier already.
	treeish := fmt.Sprintf("%s..%s", oldRevision, newRevision)
	client := &gitiles.Client{Client: &http.Client{Transport: transport}}
	commits, err := client.Log(c, repoURL, treeish)
	if err != nil {
		return nil, errors.Annotate(err, "fetching commit from Gitiles").Err()
	}
	// Sanity check.
	if commits[0].Commit != newRevision {
		return nil, fmt.Errorf("gitiles returned inconsistent results")
	}
	return commits, nil
}

// IsBuildAllowed returns true if luci-notify is allowed to handle b.
func isBuildAllowed(b *buildbucket.Build) bool {
	// TODO(mknyszek): Do a real ACL check here on whether the service should
	// be allowed to process the build. This is a conservative solution for now
	// which ensures that the build is public.
	tags := strpair.ParseMap(b.Tags["swarming_tag"])
	return tags.Get("allow_milo") == "1"
}

func getBuilderID(b *buildbucket.Build) string {
	return fmt.Sprintf("buildbucket/%s/%s", b.Bucket, b.Builder)
}

func commitIndex(commits []gitiles.Commit, revision string) int {
	index := -1
	for i, commit := range commits {
		if commit.Commit == revision {
			index = i
			break
		}
	}
	return index
}

// getOrInitBuilder attempts to get a Builder from the datastore, and if none exists, initializes one
// using the current build.
//
// In the case that the Builder is updated, this function returns datastore.ErrNoSuchEntity as well
// as a dummy Builder with a Status of StatusUnknown.
func getOrInitBuilder(c context.Context, id, revision string, build *buildbucket.Build) (*Builder, error) {
	updated := false
	builder := &Builder{ID: id}
	switch err := datastore.Get(c, builder); {
	case err == datastore.ErrNoSuchEntity:
		// If the Builder isn't found, start a transaction and try to store
		// the builder for the first time.
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			switch err := datastore.Get(c, builder); {
			case err == datastore.ErrNoSuchEntity:
				updated = true
				return datastore.Put(c, NewBuilder(id, revision, build))
			case err != nil:
				return err
			}
			// If we actually managed to get a Builder, we should continue with the
			// full code path.
			return nil
		}, nil)
		if err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}
	if updated {
		builder.Status = StatusUnknown
		return builder, datastore.ErrNoSuchEntity
	}
	return builder, nil
}

// handleBuild processes a build recieved via HTTP request (PubSub).
//
// This function should serve as documentation of the process of going from
// HTTP request to sent notifications. It also should explicitly handle ACLs and
// stop the process of handling notifications early to avoid wasting compute time.
//
// Errors should only be propagated if they prevent forward progress. If they do not,
// simply log the error and continue.
func handleBuild(c context.Context, r *http.Request) error {
	build, err := ExtractBuild(c, r)
	switch {
	case err != nil:
		return err
	case !isBuildAllowed(build):
		return errAccessDenied
	case !strings.HasPrefix(build.Bucket, "luci."):
		logging.Infof(c, "Received build that isn't part of LUCI, ignoring...")
		return nil
	case !build.Status.Completed():
		logging.Infof(c, "Received build that hasn't completed yet, ignoring...")
		return nil
	}

	builderID := getBuilderID(build)
	logging.Infof(c, "Finding config for %q, %s", builderID, build.Status)
	notifiers, err := config.LookupNotifiers(c, builderID)
	if err != nil {
		return errors.Annotate(err, "looking up notifiers").Err()
	}
	if len(notifiers) == 0 {
		logging.Infof(c, "No configuration was found for this builder, ignoring...")
		return nil
	}

	gCommit := getCommitBuildSet(build.BuildSets)
	if gCommit == nil {
		logging.Infof(c, "No revision information found for this build, ignoring...")
		return nil
	}

	builder, err := getOrInitBuilder(c, builderID, gCommit.Revision, build)
	switch {
	case err == datastore.ErrNoSuchEntity:
		return Notify(c, notifiers, build, builder)
	case err != nil:
		return err
	}

	// commits contains a list of commits from gCommit.Revision..builder.StatusRevision (newest..oldest).
	commits, err := getCommitHistory(c, gCommit.ProjectURL(), gCommit.Revision, builder.StatusRevision)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		logging.Debugf(c, "Found build with old commit, ignoring...")
		return nil
	}

	// Get builder, determine order, and update state.
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, builder); {
		case err == datastore.ErrNoSuchEntity:
			return errBuilderDeleted
		case err != nil:
			return err
		}
		index := commitIndex(commits, builder.StatusRevision)
		switch {
		// If the revision is not found, we can conclude that the Builder has
		// advanced beyond gCommit.Revision. This is because:
		// 1) builder.StatusRevision only ever moves forward.
		// 2) commits contains the git history up to gCommit.Revision.
		case index < 0:
			logging.Debugf(c, "Found build with old commit, ignoring...")
			return nil

		// If the revision is current, check build creation time.
		case index == 0 && builder.StatusTime.After(build.CreationTime):
			logging.Debugf(c, "Found build with the same commit but an old time, ignoring...")
			return nil
		}
		return datastore.Put(c, NewBuilder(builderID, gCommit.Revision, build))
	}, nil)
	if err != nil {
		return err
	}

	// FIXME(mknyszek): if email sending fails and if retried, the builder status
	// is already updated so next time this code runs, OnChange notification won't
	// be sent.
	// TODO(mknyszek): if a notification needs to be sent, create a push task for sending
	// in the builder's transaction, instead of sending right away.
	return Notify(c, notifiers, build, builder)
}

// BuildbucketPubSubHandler is the main entrypoint for a new update from buildbucket's pubsub.
//
// This handler delegates the actual processing of the build to handleBuild.
// Its primary purpose is to unwrap context boilerplate and deal with progress-stopping errors.
func BuildbucketPubSubHandler(ctx *router.Context) {
	c, h, r := ctx.Context, ctx.Writer, ctx.Request
	if err := handleBuild(c, r); err != nil {
		logging.WithError(err).Errorf(c, "error while notifying")
		if transient.Tag.In(err) {
			// Transient errors are 500 so that PubSub retries them.
			h.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	h.WriteHeader(http.StatusOK)
}

// ExtractBuild constructs a Build from the PubSub HTTP request.
func ExtractBuild(c context.Context, r *http.Request) (*buildbucket.Build, error) {
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
	var build buildbucket.Build
	if err := build.ParseMessage(&message.Build); err != nil {
		return nil, errors.Annotate(err, "could not decode buildbucket build").Err()
	}
	return &build, nil
}
