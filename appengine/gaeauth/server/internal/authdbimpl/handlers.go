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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/gae/service/info"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/server/auth/service"
	"github.com/luci/luci-go/server/router"
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
func setupPubSub(c context.Context, baseURL, authServiceURL string) error {
	pushURL := ""
	if !info.IsDevAppServer(c) {
		pushURL = baseURL + pubSubPushURLPath // push in prod, pull on dev server
	}
	service := getAuthService(c, authServiceURL)
	return service.EnsureSubscription(c, subscriptionName(c, authServiceURL), pushURL)
}

// killPubSub removes PubSub subscription created with setupPubSub.
func killPubSub(c context.Context, authServiceURL string) error {
	service := getAuthService(c, authServiceURL)
	return service.DeleteSubscription(c, subscriptionName(c, authServiceURL))
}

// subscriptionName returns full PubSub subscription name for AuthDB
// change notifications stream from given auth service.
func subscriptionName(c context.Context, authServiceURL string) string {
	subIDPrefix := "gae-v1"
	if info.IsDevAppServer(c) {
		subIDPrefix = "dev-app-server-v1"
	}
	serviceURL, err := url.Parse(authServiceURL)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("projects/%s/subscriptions/%s+%s", info.AppID(c), subIDPrefix, serviceURL.Host)
}

// pubSubPull is HTTP handler that pulls PubSub messages from AuthDB change
// notification topic.
//
// Used only on dev server for manual testing. Prod services use push-based
// delivery.
func pubSubPull(c *router.Context) {
	if !appengine.IsDevAppServer() {
		replyError(c.Context, c.Writer, errors.New("not a dev server"))
		return
	}
	processPubSubRequest(c.Context, c.Writer, c.Request, func(c context.Context, srv authService, serviceURL string) (*service.Notification, error) {
		return srv.PullPubSub(c, subscriptionName(c, serviceURL))
	})
}

// pubSubPush is HTTP handler that processes incoming PubSub push notifications.
//
// It uses the signature inside PubSub message body for authentication. Skips
// messages not signed by currently configured auth service.
func pubSubPush(c *router.Context) {
	processPubSubRequest(c.Context, c.Writer, c.Request, func(ctx context.Context, srv authService, serviceURL string) (*service.Notification, error) {
		body, err := ioutil.ReadAll(c.Request.Body)
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
func processPubSubRequest(c context.Context, rw http.ResponseWriter, r *http.Request, callback notifcationGetter) {
	c = defaultNS(c)
	info, err := GetLatestSnapshotInfo(c)
	if err != nil {
		replyError(c, rw, err)
		return
	}
	if info == nil {
		// Return HTTP 200 to avoid a redelivery.
		replyOK(c, rw, "Auth Service URL is not configured, skipping the message")
		return
	}
	srv := getAuthService(c, info.AuthServiceURL)

	notify, err := callback(c, srv, info.AuthServiceURL)
	if err != nil {
		replyError(c, rw, err)
		return
	}

	// notify may be nil if PubSub messages didn't pass authentication.
	if notify == nil {
		replyOK(c, rw, "No new valid AuthDB change notifications")
		return
	}

	// Don't bother processing late messages (ack them though).
	latest := info
	if notify.Revision > info.Rev {
		var err error
		if latest, err = syncAuthDB(c); err != nil {
			replyError(c, rw, err)
			return
		}
	}

	if err := notify.Acknowledge(c); err != nil {
		replyError(c, rw, err)
		return
	}

	replyOK(
		c, rw, "Processed PubSub notification for rev %d: %d -> %d",
		notify.Revision, info.Rev, latest.Rev)
}

// replyError sends HTTP 500 on transient errors, HTTP 400 on fatal ones.
func replyError(c context.Context, rw http.ResponseWriter, err error) {
	logging.Errorf(c, "Error while processing PubSub notification - %s", err)
	if transient.Tag.In(err) {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	} else {
		http.Error(rw, err.Error(), http.StatusBadRequest)
	}
}

// replyOK sends HTTP 200.
func replyOK(c context.Context, rw http.ResponseWriter, msg string, args ...interface{}) {
	logging.Infof(c, msg, args...)
	rw.Write([]byte(fmt.Sprintf(msg, args...)))
}
