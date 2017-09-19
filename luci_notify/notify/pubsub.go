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
	"errors"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/luci_notify/buildbucket"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/server/router"
)

var errAccessDenied = errors.New("access to build denied")

// handleBuild processes a build recieved via HTTP request (PubSub).
//
// This function should be serve as documentation of the process of going from
// HTTP request to sent notifications. It also should explicitly handle ACLs and
// stopping the process of handling notifications early to avoid wasting compute time.
//
// Errors should only be propagated if they prevent forward progress. If they do not,
// simply log the error and continue.
func handleBuild(c context.Context, r *http.Request) error {
	build, err := buildbucket.ExtractBuildInfo(c, r)
	if err != nil {
		return err
	}
	if !build.IsAllowed() {
		return errAccessDenied
	}
	if !build.IsLUCI() {
		logging.Infof(c, "Recieved build that isn't part of LUCI, ignoring...")
		return nil
	}
	if build.Build.Status != "COMPLETED" {
		logging.Infof(c, "Recieved build that hasn't completed yet, ignoring...")
		return nil
	}
	logging.Infof(c, "Finding config for %s, %s", build.GetBuilderID(), build.Build.Status)

	notifiers := config.LookupNotifiers(c, build)
	if len(notifiers) == 0 {
		logging.Infof(c, "No configuration was found for this builder, ignoring...")
		return nil
	}
	state := LookupBuilderState(c, build)
	notification := CreateNotification(notifiers, build, state)
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
