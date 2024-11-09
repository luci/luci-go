// Copyright 2015 The LUCI Authors.
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

package authdbimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"google.golang.org/appengine"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/router"
)

const (
	pubSubPullURLPath = "/auth/pubsub/authdb:pull" // dev server only
	pubSubPushURLPath = "/auth/pubsub/authdb:push"
)

// InstallHandlers installs PubSub related HTTP handlers.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	if appengine.IsDevAppServer() {
		r.GET(pubSubPullURLPath, base, pubSubPull)
	}
	r.POST(pubSubPushURLPath, base, pubSubPush)
}

// setupPubSub creates a subscription to AuthDB service notification stream.
func setupPubSub(ctx context.Context, baseURL, authServiceURL string) error {
	pushURL := ""
	if !info.IsDevAppServer(ctx) {
		pushURL = baseURL + pubSubPushURLPath // push in prod, pull on dev server
	}
	service := getAuthService(ctx, authServiceURL)
	return service.EnsureSubscription(ctx, subscriptionName(ctx, authServiceURL), pushURL)
}

// killPubSub removes PubSub subscription created with setupPubSub.
func killPubSub(ctx context.Context, authServiceURL string) error {
	service := getAuthService(ctx, authServiceURL)
	return service.DeleteSubscription(ctx, subscriptionName(ctx, authServiceURL))
}

// subscriptionName returns full PubSub subscription name for AuthDB
// change notifications stream from given auth service.
func subscriptionName(ctx context.Context, authServiceURL string) string {
	subIDPrefix := "gae-v1"
	if info.IsDevAppServer(ctx) {
		subIDPrefix = "dev-app-server-v1"
	}
	serviceURL, err := url.Parse(authServiceURL)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("projects/%s/subscriptions/%s+%s", info.AppID(ctx), subIDPrefix, serviceURL.Host)
}

// pubSubPull is HTTP handler that pulls PubSub messages from AuthDB change
// notification topic.
//
// Used only on dev server for manual testing. Prod services use push-based
// delivery.
func pubSubPull(c *router.Context) {
	if !appengine.IsDevAppServer() {
		replyError(c.Request.Context(), c.Writer, errors.New("not a dev server"))
		return
	}
	processPubSubRequest(c.Request.Context(), c.Writer, c.Request, func(ctx context.Context, srv authService, serviceURL string) (*service.Notification, error) {
		return srv.PullPubSub(ctx, subscriptionName(ctx, serviceURL))
	})
}

// pubSubPush is HTTP handler that processes incoming PubSub push notifications.
//
// It uses the signature inside PubSub message body for authentication. Skips
// messages not signed by currently configured auth service.
func pubSubPush(c *router.Context) {
	processPubSubRequest(c.Request.Context(), c.Writer, c.Request, func(ctx context.Context, srv authService, serviceURL string) (*service.Notification, error) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, err
		}
		return srv.ProcessPubSubPush(ctx, body)
	})
}

type notifcationGetter func(context.Context, authService, string) (*service.Notification, error)

// processPubSubRequest is common wrapper for pubSubPull and pubSubPush.
//
// It implements most logic of notification handling. Calls supplied callback
// to actually get service.Notification, since this part is different from Pull
// and Push subscriptions.
func processPubSubRequest(ctx context.Context, rw http.ResponseWriter, r *http.Request, callback notifcationGetter) {
	ctx = defaultNS(ctx)
	info, err := GetLatestSnapshotInfo(ctx)
	if err != nil {
		replyError(ctx, rw, err)
		return
	}
	if info == nil {
		// Return HTTP 200 to avoid a redelivery.
		replyOK(ctx, rw, "Auth Service URL is not configured, skipping the message")
		return
	}
	srv := getAuthService(ctx, info.AuthServiceURL)

	notify, err := callback(ctx, srv, info.AuthServiceURL)
	if err != nil {
		replyError(ctx, rw, err)
		return
	}

	// notify may be nil if PubSub messages didn't pass authentication.
	if notify == nil {
		replyOK(ctx, rw, "No new valid AuthDB change notifications")
		return
	}

	// Don't bother processing late messages (ack them though).
	latest := info
	if notify.Revision > info.Rev {
		var err error
		if latest, err = syncAuthDB(ctx); err != nil {
			replyError(ctx, rw, err)
			return
		}
	}

	if err := notify.Acknowledge(ctx); err != nil {
		replyError(ctx, rw, err)
		return
	}

	replyOK(
		ctx, rw, "Processed PubSub notification for rev %d: %d -> %d",
		notify.Revision, info.Rev, latest.Rev)
}

// replyError sends HTTP 500 on transient errors, HTTP 400 on fatal ones.
func replyError(ctx context.Context, rw http.ResponseWriter, err error) {
	logging.Errorf(ctx, "Error while processing PubSub notification - %s", err)
	if transient.Tag.In(err) {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	} else {
		http.Error(rw, err.Error(), http.StatusBadRequest)
	}
}

// replyOK sends HTTP 200.
func replyOK(ctx context.Context, rw http.ResponseWriter, msg string, args ...any) {
	logging.Infof(ctx, msg, args...)
	rw.Write([]byte(fmt.Sprintf(msg, args...)))
}
