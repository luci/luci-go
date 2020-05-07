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

package main

import (
	"context"
	"net/http"

	"google.golang.org/appengine"

	authServer "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/notify"
)

var buildbucketPubSub = metric.NewCounter(
	"luci/notify/buildbucket-pubsub",
	"Number of received Buildbucket PubSub messages",
	nil,
	// "success", "transient-failure" or "permanent-failure"
	field.String("status"),
)

func main() {
	r := router.New()
	standard.InstallHandlers(r)

	basemw := standard.Base().Extend(withRemoteConfigService)
	// TODO(crbug.com/1079092): Remove remaining uses of CookieAuth.
	cookieAuthMw := basemw.Extend(auth.Authenticate(authServer.CookieAuth))

	taskDispatcher := tq.Dispatcher{BaseURL: "/internal/tasks/"}
	notify.InitDispatcher(&taskDispatcher)
	taskDispatcher.InstallRoutes(r, cookieAuthMw)

	// Cron endpoints.
	r.GET("/internal/cron/update-config", basemw.Extend(gaemiddleware.RequireCron), config.UpdateHandler)
	r.GET("/internal/cron/update-tree-status", basemw.Extend(gaemiddleware.RequireCron), notify.UpdateTreeStatus)

	// Pub/Sub endpoint.
	r.POST("/_ah/push-handlers/buildbucket", cookieAuthMw, func(c *router.Context) {
		ctx, cancel := context.WithTimeout(c.Context, notify.PUBSUB_POST_REQUEST_TIMEOUT)
		defer cancel()
		c.Context = ctx

		status := ""
		switch err := notify.BuildbucketPubSubHandler(c, &taskDispatcher); {
		case transient.Tag.In(err) || appengine.IsTimeoutError(errors.Unwrap(err)):
			status = "transient-failure"
			logging.Errorf(ctx, "transient failure: %s", err)
			// Retry the message.
			c.Writer.WriteHeader(http.StatusInternalServerError)

		case err != nil:
			status = "permanent-failure"
			logging.Errorf(ctx, "permanent failure: %s", err)

		default:
			status = "success"
		}

		buildbucketPubSub.Add(ctx, 1, status)
	})

	http.Handle("/", r)
	appengine.Main()
}

func withRemoteConfigService(c *router.Context, next router.Handler) {
	s, err := gaeconfig.FetchCachedSettings(c.Context)
	if err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		logging.WithError(err).Errorf(c.Context, "failure retrieving cached settings")
		return
	}

	rInterface := remote.New(s.ConfigServiceHost, false, func(c context.Context) (*http.Client, error) {
		t, err := auth.GetRPCTransport(c, auth.AsSelf)
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	})
	// insert into context
	c.Context = config.WithConfigService(c.Context, rInterface)
	next(c)
}
