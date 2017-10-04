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

	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/server/router"
)

var errAccessDenied = fmt.Errorf("access to build denied")

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

	// LookupBuilder updates the datastore, so don't do this until we've already
	// looked up notifiers to avoid tracking state for a builder we don't have a
	// notifier for.
	builder, err := LookupBuilder(c, builderID, build)
	if err != nil {
		return errors.Annotate(err, "looking up builder").Err()
	}
	logging.Debugf(c, "Got state: %v", builder)

	notification := CreateNotification(c, notifiers, build, builder)
	if notification == nil {
		logging.Infof(c, "Not notifying anybody...")
		return nil
	}
	// FIXME(mknyszek): if email sending fails
	// 1) a non-transient error is returned and a notification is not sent
	// 2) even if retried, the builder status  is already updated
	//    so next time this code runs, OnChanged notification won't be sent
	// TODO: if a notification needs to be sent, create a push task for sending
	// in the builder's transaction, instead of sending right away.
	return notification.Dispatch(c)
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
