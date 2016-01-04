// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"net/http"

	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"
	gOAuth "golang.org/x/oauth2/google"
	"google.golang.org/appengine"
	"google.golang.org/appengine/remote_api"
)

type key int

var (
	prodContextKey      key
	prodContextNoTxnKey key = 1
	probeCacheKey       key = 2
)

// AEContext retrieves the raw "google.golang.org/appengine" compatible Context.
//
// It also transfers deadline of `c` to AE context, since deadline is used for
// RPCs. Doesn't transfer cancelation ability though (since it's ignored by GAE
// anyway).
func AEContext(c context.Context) context.Context {
	aeCtx, _ := c.Value(prodContextKey).(context.Context)
	if deadline, ok := c.Deadline(); ok {
		aeCtx, _ = context.WithDeadline(aeCtx, deadline)
	}
	return aeCtx
}

// AEContextNoTxn retrieves the raw "google.golang.org/appengine" compatible
// Context that's not part of a transaction.
func AEContextNoTxn(c context.Context) context.Context {
	aeCtx, _ := c.Value(prodContextNoTxnKey).(context.Context)
	aeCtx, err := appengine.Namespace(aeCtx, info.Get(c).GetNamespace())
	if err != nil {
		panic(err)
	}
	if deadline, ok := c.Deadline(); ok {
		aeCtx, _ = context.WithDeadline(aeCtx, deadline)
	}
	return aeCtx
}

func setupAECtx(c, aeCtx context.Context) context.Context {
	c = context.WithValue(c, prodContextKey, aeCtx)
	c = context.WithValue(c, prodContextNoTxnKey, aeCtx)
	return useMail(useUser(useURLFetch(useRDS(useMC(useTQ(useGI(useLogging(c))))))))
}

// Use adds production implementations for all the gae services to the
// context.
//
// The services added are:
//   - github.com/luci-go/common/logging
//   - github.com/luci/gae/service/datastore
//   - github.com/luci/gae/service/info
//   - github.com/luci/gae/service/mail
//   - github.com/luci/gae/service/memcache
//   - github.com/luci/gae/service/taskqueue
//   - github.com/luci/gae/service/urlfetch
//   - github.com/luci/gae/service/user
//
// These can be retrieved with the <service>.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func Use(c context.Context, r *http.Request) context.Context {
	return setupAECtx(c, appengine.NewContext(r))
}

// UseRemote is the same as Use, except that it lets you attach a context to
// a remote host using the Remote API feature. See the docs for the
// prerequisites.
//
// docs: https://cloud.google.com/appengine/docs/go/tools/remoteapi
//
// If client is nil, this will use a default Google OAuth2 client. Otherwise the
// client must be configured to have the following OAuth2 scopes:
//		"https://www.googleapis.com/auth/appengine.apis"
//		"https://www.googleapis.com/auth/userinfo.email"
//		"https://www.googleapis.com/auth/cloud.platform"
func UseRemote(c context.Context, host string, client *http.Client) (context.Context, error) {
	err := error(nil)
	if client == nil {
		client, err = gOAuth.DefaultClient(context.Background(),
			"https://www.googleapis.com/auth/appengine.apis",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/cloud.platform",
		)
		if err != nil {
			return nil, err
		}
	}

	aeCtx, err := remote_api.NewRemoteContext(host, client)
	if err != nil {
		return nil, err
	}
	return setupAECtx(c, aeCtx), nil
}
