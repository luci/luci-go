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

package remote

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/config"
	. "github.com/smartystreets/goconvey/convey"
)

func encodeToB(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func testTools(code int, resp interface{}) (*httptest.Server, config.Interface) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		marsh, _ := json.Marshal(resp)
		fmt.Fprintln(w, string(marsh))
	}))

	u, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	return server, New(u.Host, true, nil)
}

func TestRemoteCalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Should pass through calls to the generated API", t, func() {
		Convey("GetConfig", func() {
			server, remoteImpl := testTools(200, map[string]string{
				"content":      encodeToB("hi"),
				"content_hash": "bar",
				"revision":     "3",
			})
			defer server.Close()

			res, err := remoteImpl.GetConfig(ctx, "a", "b", false)

			So(err, ShouldBeNil)
			So(*res, ShouldResemble, config.Config{
				ConfigSet:   "a",
				Path:        "b",
				Content:     "hi",
				ContentHash: "bar",
				Revision:    "3",
			})
		})
		Convey("GetConfigByHash", func() {
			server, remoteImpl := testTools(200, map[string]string{
				"content": encodeToB("content"),
			})
			defer server.Close()

			res, err := remoteImpl.GetConfigByHash(ctx, "a")

			So(err, ShouldBeNil)
			So(res, ShouldResemble, "content")
		})
		Convey("GetConfigSetLocation", func() {
			URL, err := url.Parse("http://example.com")
			if err != nil {
				panic(err)
			}

			server, remoteImpl := testTools(200, map[string]interface{}{
				"config_sets": [...]interface{}{map[string]string{
					"config_set": "a",
					"location":   URL.String(),
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetConfigSetLocation(ctx, "a")

			So(err, ShouldBeNil)
			So(*res, ShouldResemble, *URL)
		})
		Convey("GetProjectConfigs", func() {
			server, remoteImpl := testTools(200, map[string]interface{}{
				"configs": [...]interface{}{map[string]string{
					"config_set":   "a",
					"content":      encodeToB("hi"),
					"content_hash": "bar",
					"revision":     "3",
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetProjectConfigs(ctx, "b", false)

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldResemble, config.Config{
				ConfigSet:   "a",
				Path:        "b",
				Content:     "hi",
				ContentHash: "bar",
				Revision:    "3",
			})
		})
		Convey("GetProjectConfigs hashesOnly", func() {
			server, remoteImpl := testTools(200, map[string]interface{}{
				"configs": [...]interface{}{map[string]string{
					"config_set":   "a",
					"content_hash": "bar",
					"revision":     "3",
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetProjectConfigs(ctx, "b", true)

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldResemble, config.Config{
				ConfigSet:   "a",
				Path:        "b",
				Content:     "",
				ContentHash: "bar",
				Revision:    "3",
			})
		})
		Convey("GetProjects", func() {
			id := "blink"
			name := "Blink"
			URL, err := url.Parse("http://example.com")
			if err != nil {
				panic(err)
			}

			server, remoteImpl := testTools(200, map[string]interface{}{
				"projects": [...]interface{}{map[string]string{
					"id":        id,
					"name":      name,
					"repo_type": "GITILES",
					"repo_url":  URL.String(),
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetProjects(ctx)

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldResemble, config.Project{
				ID:       id,
				Name:     name,
				RepoType: config.GitilesRepo,
				RepoURL:  URL,
			})
		})
		Convey("GetRefConfigs", func() {
			server, remoteImpl := testTools(200, map[string]interface{}{
				"configs": [...]interface{}{map[string]string{
					"config_set":   "a",
					"content":      encodeToB("hi"),
					"content_hash": "bar",
					"revision":     "3",
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetRefConfigs(ctx, "b", false)

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldResemble, config.Config{
				ConfigSet:   "a",
				Path:        "b",
				Content:     "hi",
				ContentHash: "bar",
				Revision:    "3",
			})
		})
		Convey("GetRefConfigs hashesOnly", func() {
			server, remoteImpl := testTools(200, map[string]interface{}{
				"configs": [...]interface{}{map[string]string{
					"config_set":   "a",
					"content_hash": "bar",
					"revision":     "3",
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetRefConfigs(ctx, "b", true)

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldResemble, config.Config{
				ConfigSet:   "a",
				Path:        "b",
				ContentHash: "bar",
				Revision:    "3",
			})
		})
		Convey("GetRefs", func() {
			ref := "refs/heads/master"
			server, remoteImpl := testTools(200, map[string]interface{}{
				"refs": [...]interface{}{map[string]string{
					"name": ref,
				}},
			})
			defer server.Close()

			res, err := remoteImpl.GetRefs(ctx, "a")

			So(err, ShouldBeNil)
			So(res, ShouldNotBeEmpty)
			So(len(res), ShouldEqual, 1)
			So(res[0], ShouldEqual, ref)
		})
	})

	Convey("Should handle errors well", t, func() {
		Convey("Should enforce GetConfigSetLocation argument is not the empty string.", func() {
			remoteImpl := New("example.com", true, nil)

			_, err := remoteImpl.GetConfigSetLocation(ctx, "")
			So(err, ShouldNotBeNil)
		})

		Convey("Should pass through HTTP errors", func() {
			remoteImpl := New("example.com", true, func(context.Context) (*http.Client, error) {
				return &http.Client{
					Transport: failingRoundTripper{},
				}, nil
			})

			_, err := remoteImpl.GetConfig(ctx, "a", "b", false)
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetConfigByHash(ctx, "a")
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetConfigSetLocation(ctx, "a")
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetProjectConfigs(ctx, "a", false)
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetProjects(ctx)
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetRefConfigs(ctx, "a", false)
			So(err, ShouldNotBeNil)
			_, err = remoteImpl.GetRefs(ctx, "a")
			So(err, ShouldNotBeNil)
		})
	})
}

type failingRoundTripper struct{}

func (t failingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("IM AM ERRAR\n")
}
