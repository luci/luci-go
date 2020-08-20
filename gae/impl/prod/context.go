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

package prod

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"go.chromium.org/gae/service/urlfetch"
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

// getAEContext retrieves the raw "google.golang.org/appengine" compatible
// Context.
//
// This is an independent Context chain from `c`. In an attempt to maintain user
// expectations, the deadline of `c` is transferred to the returned Context,
// RPCs. Cancelation is not transferred.
func getAEContext(c context.Context) context.Context {
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
// context. The implementations are all backed by the real appengine SDK
// functionality.
//
// The services added are:
//   - github.com/luci-go/common/logging
//   - go.chromium.org/gae/service/datastore
//   - go.chromium.org/gae/service/info
//   - go.chromium.org/gae/service/mail
//   - go.chromium.org/gae/service/memcache
//   - go.chromium.org/gae/service/module
//   - go.chromium.org/gae/service/taskqueue
//   - go.chromium.org/gae/service/urlfetch
//   - go.chromium.org/gae/service/user
//
// These can be retrieved with the <service>.Get functions.
//
// It is important to note that this DOES NOT install the AppEngine SDK into the
// supplied Context. In general, using the raw AppEngine SDK to access a service
// that is covered by luci/gae is dangerous, leading to a number of potential
// pitfalls including inconsistent transaction management and data corruption.
//
// Users who wish to access the raw AppEngine SDK must derive their own
// AppEngine Context at their own risk.
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
//
// It is important to note that this DOES NOT install the AppEngine SDK into the
// supplied Context. See the warning in Use for more information.
func UseRemote(inOutCtx *context.Context, host string, client *http.Client) (err error) {
	if client == nil {
		aeCtx := getAEContext(*inOutCtx)

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
// https://go.chromium.org/gae/issues/59
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
