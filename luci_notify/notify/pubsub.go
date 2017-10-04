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
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/luci_notify/buildbucket"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/server/router"
)

var errAccessDenied = fmt.Errorf("access to build denied")

// handleBuild processes a build recieved via HTTP request (PubSub).
//
// This function should serve as documentation of the process of going from
// HTTP request to sent notifications. It also should explicitly handle ACLs and
// stop the process of handling notifications early to avoid wasting compute time.
//
// Errors should only be propagated if they prevent forward progress. If they do not,
// simply log the error and continue.
func handleBuild(c context.Context, r *http.Request) error {
	build, err := buildbucket.ExtractBuildInfo(c, r)
	switch {
	case err != nil:
		return err
	case !build.IsAllowed():
		return errAccessDenied
	case !build.IsLUCI():
		logging.Infof(c, "Received build that isn't part of LUCI, ignoring...")
		return nil
	case build.Build.Status != "COMPLETED":
		logging.Infof(c, "Received build that hasn't completed yet, ignoring...")
		return nil
	}

	logging.Infof(c, "Finding config for %q, %s", build.BuilderID(), build.Build.Result)
	notifiers, err := config.LookupNotifiers(c, build)
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
	builder, err := LookupBuilder(c, build)
	if err != nil {
		return errors.Annotate(err, "looking up builder").Err()
	}
	logging.Debugf(c, "Got state: %v", builder)

	notification := CreateNotification(c, notifiers, build, builder)
	if notification == nil {
		logging.Infof(c, "Not notifying anybody...")
		return nil
	}
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
