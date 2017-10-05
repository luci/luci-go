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
	errBuildOutOfOrder = fmt.Errorf("build out-of-order by creation time")
	errPreempted = fmt.Errorf("preempted by another build")
)

type Ordering int

const (
	IN_ORDER = iota
	SAME
	OUT_OF_ORDER
	UNKNOWN
)

func getCommitBuildSet(sets []buildbucket.BuildSet) *buildbucket.GitilesCommit {
	for _, set := range sets {
		if commit, ok := set.(*buildbucket.GitilesCommit); ok {
			return commit
		}
	}
	return nil
}

// getCommitOrder determines the relative ordering of the commit for a new build
// vs. some older revision.
func getCommitOrder(c context.Context, newCommit *buildbucket.GitilesCommit, oldRevision string) (Ordering, error) {
	if newCommit.Revision == oldRevision {
		return SAME, nil
	}
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(
		"https://www.googleapis.com/auth/gerritcodereview",
	))
	if err != nil {
		return UNKNOWN, errors.Annotate(err, "getting RPC Transport").Err()
	}

	// With this range, if newCommit.Revision == oldRevision we'll get nothing, but
	// this was checked earlier already.
	treeish := fmt.Sprintf("%s..%s", oldRevision, newCommit.Revision)
	client := &gitiles.Client{Client: &http.Client{Transport: transport}}
	commits, err := client.Log(c, newCommit.ProjectURL(), treeish)
	if err != nil {
		return UNKNOWN, errors.Annotate(err, "fetching commit from Gitiles").Err()
	}

	// Since they're not the same commit, we can be sure this implies that they're
	// out-of-order.
	if len(commits) == 0 {
		return OUT_OF_ORDER, nil
	}
	return IN_ORDER, nil
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

// getOrInitBuilder attempts to Get a Builder from the datastore.
// If the Builder is not found, then we Put it immediately within the same transaction.
func getOrInitBuilder(c context.Context, id, revision string, build *buildbucket.Build) (*Builder, error) {
	var gotBuilder *Builder
	err := WithBuilder(c, id, func(c context.Context, builder *Builder) error {
		gotBuilder = builder
		if builder.Status == StatusUnknown {
			return datastore.Put(c, NewBuilder(id, revision, build))
		}
		return nil
	})
	return gotBuilder, err
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

	currentBuilder, err := getOrInitBuilder(c, builderID, gCommit.Revision, build)
	if err != nil {
		return errors.Annotate(err, "failed in first builder transaction").Err()
	}
	// If this builder wasn't found, getBuilder updated it already so we're done.
	if currentBuilder.Status == StatusUnknown {
		logging.Infof(c, "Registered new builder: %s", builderID)
		return nil
	}

	// Here we loop until we can manage to actually land a change without getting
	// pre-empted by another instance trying to land a change for the same builder.
	// This is necessary because between the last Get where currentBuilder was
	// received and now, there could have been a change to the datastore state.
	var prevBuilder *Builder
	for {
		var commitOrder Ordering
		if prevBuilder == nil || prevBuilder.StatusRevision != currentBuilder.StatusRevision {
			commitOrder, err = getCommitOrder(c, gCommit, currentBuilder.StatusRevision)
			if err != nil {
				return err
			}
			switch commitOrder {
			case SAME:
				// continue
			case IN_ORDER:
				// continue
			case OUT_OF_ORDER:
				logging.Debugf(c, "Found build with old commit, ignoring...")
				return nil
			default:
				return fmt.Errorf("found invalid Ordering")
			}
		}

		prevBuilder = currentBuilder
		err = WithBuilder(c, builderID, func(c context.Context, builder *Builder) error {
			currentBuilder = builder
			if !prevBuilder.UpdateTime.Equal(builder.UpdateTime) {
				return errPreempted
			}
			// Don't bother doing anything if the status hasn't changed.
			if build.Status != builder.Status {
				return nil
			}
			// If the revision is the same, check build creation time.
			if commitOrder == SAME && builder.StatusTime.After(build.CreationTime) {
				return errBuildOutOfOrder
			}
			return datastore.Put(c, NewBuilder(builderID, gCommit.Revision, build))
		})
		switch err {
		case errPreempted:
			continue
		case errBuildOutOfOrder:
			logging.Debugf(c, "Found build with old commit, ignoring...")
			return nil
		case nil:
			// FIXME(mknyszek): if email sending fails and if retried, the builder status
			// is already updated so next time this code runs, OnChange notification won't
			// be sent.
			// TODO(mknyszek): if a notification needs to be sent, create a push task for sending
			// in the builder's transaction, instead of sending right away.
			return Notify(c, notifiers, build, currentBuilder)
		default:
			return err
		}
	}
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
