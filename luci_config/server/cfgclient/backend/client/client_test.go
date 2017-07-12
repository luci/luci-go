// Copyright 2016 The LUCI Authors.
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

package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	configApi "github.com/luci/luci-go/common/api/luci_config/config/v1"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/delegation"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type remoteInterfaceService struct {
	err     error
	lastReq *http.Request
	res     interface{}
}

func (ris *remoteInterfaceService) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ris.lastReq = req
	if err := ris.err; err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(rw).Encode(ris.res); err != nil {
		http.Error(rw, fmt.Sprintf("failed to encode JSON: %s", err), http.StatusInternalServerError)
		return
	}
}

func TestRemoteService(t *testing.T) {
	t.Parallel()

	Convey(`Testing the remote service`, t, func() {
		c := context.Background()

		fs := authtest.FakeState{
			Identity: "user:foo@bar.baz",
		}
		c = auth.WithState(c, &fs)
		c = authtest.MockAuthConfig(c)

		ris := remoteInterfaceService{}
		svr := httptest.NewServer(&ris)
		defer svr.Close()

		svrURL, err := url.Parse(svr.URL)
		if err != nil {
			panic(err)
		}

		c = backend.WithBackend(c, &Backend{
			Provider: &RemoteProvider{
				Host:                    svrURL.Host,
				Insecure:                true,
				testUserDelegationToken: "user token",
			},
		})

		b64 := func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) }

		Convey(`Can get the service URL`, func() {
			hostURL, err := url.Parse(svr.URL)
			if err != nil {
				panic(err)
			}
			hostURL.Path = "/_ah/api/config/v1/"

			So(cfgclient.ServiceURL(c), ShouldResemble, *hostURL)
		})

		Convey(`Can resolve a single config`, func() {
			ris.res = configApi.LuciConfigGetConfigResponseMessage{
				Content:     b64("ohai"),
				ContentHash: "####",
				Revision:    "v1",
			}

			Convey(`AsService`, func() {
				var val string
				So(cfgclient.Get(c, cfgclient.AsService, "foo/bar", "baz", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "ohai")

				So(ris.lastReq.URL.Path, ShouldEqual, "/_ah/api/config/v1/config_sets/foo/bar/config/baz")
				So(ris.lastReq.Header.Get("Authorization"), ShouldEqual, "Bearer fake_token")
				So(ris.lastReq.Header.Get(delegation.HTTPHeaderName), ShouldEqual, "")
			})

			Convey(`AsUser`, func() {
				var val string
				So(cfgclient.Get(c, cfgclient.AsUser, "foo/bar", "baz", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "ohai")

				So(ris.lastReq.URL.Path, ShouldEqual, "/_ah/api/config/v1/config_sets/foo/bar/config/baz")
				So(ris.lastReq.Header.Get("Authorization"), ShouldEqual, "Bearer fake_token")
				So(ris.lastReq.Header.Get(delegation.HTTPHeaderName), ShouldEqual, "user token")
			})

			Convey(`AsAnonymous`, func() {
				var val string
				So(cfgclient.Get(c, cfgclient.AsAnonymous, "foo/bar", "baz", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "ohai")

				So(ris.lastReq.URL.Path, ShouldEqual, "/_ah/api/config/v1/config_sets/foo/bar/config/baz")
				So(ris.lastReq.Header.Get("Authorization"), ShouldEqual, "")
				So(ris.lastReq.Header.Get(delegation.HTTPHeaderName), ShouldEqual, "")
			})
		})

		Convey(`Can resolve multiple configs`, func() {
			ris.res = configApi.LuciConfigGetConfigMultiResponseMessage{
				Configs: []*configApi.LuciConfigGetConfigMultiResponseMessageConfigEntry{
					{ConfigSet: "projects/foo", Content: b64("foo"), ContentHash: "####", Revision: "v1"},
					{ConfigSet: "projects/bar", Content: b64("bar"), ContentHash: "####", Revision: "v1"},
				},
			}

			for _, tc := range []struct {
				name string
				fn   func(context.Context, cfgclient.Authority, string, cfgclient.MultiResolver, *[]*cfgclient.Meta) error
				path string
			}{
				{"Project", cfgclient.Projects, "/_ah/api/config/v1/configs/projects/baz"},
				{"Ref", cfgclient.Refs, "/_ah/api/config/v1/configs/refs/baz"},
			} {
				Convey(tc.name, func() {
					var (
						val  []string
						meta []*cfgclient.Meta
					)
					So(tc.fn(c, cfgclient.AsService, "baz", cfgclient.StringSlice(&val), &meta), ShouldBeNil)
					So(val, ShouldResemble, []string{"foo", "bar"})
					So(meta, ShouldResemble, []*cfgclient.Meta{
						{"projects/foo", "baz", "####", "v1"},
						{"projects/bar", "baz", "####", "v1"},
					})

					So(ris.lastReq.URL.Path, ShouldEqual, tc.path)
				})
			}
		})
	})
}

func TestAuthorityRPCTranslation(t *testing.T) {
	t.Parallel()

	Convey(`Testing authority RPC kind translation`, t, func() {
		So(rpcAuthorityKind(backend.AsAnonymous), ShouldEqual, auth.NoAuth)
		So(rpcAuthorityKind(backend.AsUser), ShouldEqual, auth.AsUser)
		So(rpcAuthorityKind(backend.AsService), ShouldEqual, auth.AsSelf)
		So(func() { rpcAuthorityKind(backend.Authority(-1)) }, ShouldPanic)
	})
}
