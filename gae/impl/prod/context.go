// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package prod

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/luci/gae/service/urlfetch"
	"golang.org/x/net/context"
	gOAuth "golang.org/x/oauth2/google"
	"google.golang.org/appengine"
	"google.golang.org/appengine/remote_api"
)

// RemoteAPIScopes is the set of OAuth2 scopes needed for Remote API access.
var RemoteAPIScopes = []string{
	"https://www.googleapis.com/auth/appengine.apis",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/cloud.platform",
}

type key int

var (
	prodStateKey  = "contains the current *prodState"
	probeCacheKey = "contains the current *infoProbeCache"
)

// AEContext retrieves the raw "google.golang.org/appengine" compatible Context.
//
// It also transfers deadline of `c` to AE context, since deadline is used for
// RPCs. Doesn't transfer cancelation ability though (since it's ignored by GAE
// anyway).
func AEContext(c context.Context) context.Context {
	ps := getProdState(c)
	return ps.context(c)
}

func setupAECtx(c, aeCtx context.Context) context.Context {
	c = withProdState(c, prodState{
		ctx:      aeCtx,
		noTxnCtx: aeCtx,
	})
	return useModule(useMail(useUser(useURLFetch(useRDS(useMC(useTQ(useGI(useLogging(c)))))))))
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
//   - github.com/luci/gae/service/module
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
// inOutCtx will be replaced with the new, derived context, if err is nil,
// otherwise it's unchanged and continues to be safe-to-use.
//
// If client is nil, this will use create a new client, and will try to be
// clever about it:
//   * If you're creating a remote context FROM AppEngine, this will use
//     urlfetch.Transport. This can be used to allow app-to-app remote_api
//     control.
//
//   * If host starts with "localhost", this will create a regular http.Client
//     with a cookiejar, and call the _ah/login API to log in as an admin with
//     the user "admin@example.com".
//
//   * Otherwise, it will create a Google OAuth2 client with the following scopes:
//       - "https://www.googleapis.com/auth/appengine.apis"
//       - "https://www.googleapis.com/auth/userinfo.email"
//       - "https://www.googleapis.com/auth/cloud.platform"
func UseRemote(inOutCtx *context.Context, host string, client *http.Client) (err error) {
	if client == nil {
		aeCtx := AEContext(*inOutCtx)

		if strings.HasPrefix(host, "localhost") {
			transp := http.DefaultTransport
			if aeCtx != nil {
				transp = urlfetch.Get(*inOutCtx)
			}

			client = &http.Client{Transport: transp}
			client.Jar, err = cookiejar.New(nil)
			if err != nil {
				return
			}
			u := fmt.Sprintf("http://%s/_ah/login?%s", host, url.Values{
				"email":  {"admin@example.com"},
				"admin":  {"True"},
				"action": {"Login"},
			}.Encode())

			var rsp *http.Response
			rsp, err = client.Get(u)
			if err != nil {
				return
			}
			defer rsp.Body.Close()
		} else {
			if aeCtx == nil {
				aeCtx = context.Background()
			}
			client, err = gOAuth.DefaultClient(aeCtx, RemoteAPIScopes...)
			if err != nil {
				return
			}
		}
	}

	aeCtx, err := remote_api.NewRemoteContext(host, client)
	if err != nil {
		return
	}
	*inOutCtx = setupAECtx(*inOutCtx, aeCtx)
	return nil
}

// prodState is the current production state.
type prodState struct {
	// ctx is the current derived GAE context.
	ctx context.Context

	// noTxnCtx is a Context maintained alongside ctx. When a transaction is
	// entered, ctx will be updated, but noTxnCtx will not, allowing extra-
	// transactional Context access.
	noTxnCtx context.Context

	// inTxn if true if this is in a transaction, false otherwise.
	inTxn bool
}

func getProdState(c context.Context) prodState {
	if v := c.Value(&prodStateKey).(*prodState); v != nil {
		return *v
	}
	return prodState{}
}

func withProdState(c context.Context, ps prodState) context.Context {
	return context.WithValue(c, &prodStateKey, &ps)
}

// context returns the current AppEngine-bound Context. Prior to returning,
// the deadline from "c" (if any) is applied.
//
// Note that this does not (currently) apply any other Done state or propagate
// cancellation from "c".
//
// Tracking at:
// https://github.com/luci/gae/issues/59
func (ps *prodState) context(c context.Context) context.Context {
	aeCtx := ps.ctx
	if aeCtx == nil {
		return nil
	}

	if deadline, ok := c.Deadline(); ok {
		aeCtx, _ = context.WithDeadline(aeCtx, deadline)
	}
	return aeCtx
}
