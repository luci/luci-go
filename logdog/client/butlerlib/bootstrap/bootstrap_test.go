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

package bootstrap

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type sentinelClient struct{}

func (sc *sentinelClient) NewStream(f streamproto.Flags) (streamclient.Stream, error) {
	return &sentinelStream{props: f.Properties()}, nil
}

type sentinelStream struct {
	streamclient.Stream
	props *streamproto.Properties
}

func (ss *sentinelStream) Properties() *streamproto.Properties { return ss.props }
func (ss *sentinelStream) Close() error                        { return nil }

func TestBootstrap(t *testing.T) {
	t.Parallel()

	Convey(`A test Environment`, t, func() {
		reg := &streamclient.Registry{}
		regSpec := ""
		regErr := error(nil)
		reg.Register("test", func(spec string, ns types.StreamName) (streamclient.Client, error) {
			regSpec = spec
			return &sentinelClient{}, regErr
		})

		env := environ.New([]string{
			"IRRELEVANT=VALUE",
		})

		Convey(`With no Butler values will return ErrNotBootstrapped.`, func() {
			_, err := getFromEnv(env, reg)
			So(err, ShouldEqual, ErrNotBootstrapped)
		})

		Convey(`With a Butler project and prefix`, func() {
			env.Set(EnvStreamProject, "test-project")
			env.Set(EnvStreamPrefix, "butler/prefix")

			Convey(`Yields a Bootstrap with a Project, Prefix, and no Client.`, func() {
				bs, err := getFromEnv(env, reg)
				So(err, ShouldBeNil)

				So(bs, ShouldResemble, &Bootstrap{
					Project: "test-project",
					Prefix:  "butler/prefix",
				})
			})

			Convey(`And the remaining environment parameters`, func() {
				env.Set(EnvStreamServerPath, "test:client:params")
				env.Set(EnvCoordinatorHost, "example.appspot.com")
				env.Set(EnvNamespace, "some/namespace")

				Convey(`Yields a fully-populated Bootstrap.`, func() {
					bs, err := getFromEnv(env, reg)
					So(err, ShouldBeNil)

					// Check that the client is populated, so we can test the remaining
					// fields without reconstructing it.
					So(bs.Client, ShouldHaveSameTypeAs, &sentinelClient{})
					bs.Client = nil

					So(bs, ShouldResemble, &Bootstrap{
						CoordinatorHost: "example.appspot.com",
						Project:         "test-project",
						Prefix:          "butler/prefix",
						Namespace:       "some/namespace",
					})
					So(regSpec, ShouldEqual, "client:params")
				})

				Convey(`If Client creation fails, will fail.`, func() {
					regErr = errors.New("testing error")
					_, err := getFromEnv(env, reg)
					So(err, ShouldErrLike, "failed to create stream client")
				})
			})

			Convey(`With an invalid Butler prefix, will fail.`, func() {
				env.Set(EnvStreamPrefix, "_notavaildprefix")
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate prefix")
			})

			Convey(`With a missing Butler project, will fail.`, func() {
				env.Remove(EnvStreamProject)
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate project")
			})

			Convey(`With an invalid Namespace, will fail.`, func() {
				env.Set(EnvNamespace, "!!! invalid")
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate namespace")
			})

			Convey(`With an invalid Butler project, will fail.`, func() {
				env.Set(EnvStreamProject, "_notavaildproject")
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate project")
			})
		})
	})
}

func TestBootstrapURLGeneration(t *testing.T) {
	t.Parallel()

	Convey(`A bootstrap instance`, t, func() {
		bs := &Bootstrap{
			Project:         "test",
			Prefix:          "foo",
			CoordinatorHost: "example.appspot.com",
		}

		Convey(`Can generate viewer URLs`, func() {
			for _, tc := range []struct {
				paths []types.StreamPath
				url   string
			}{
				{[]types.StreamPath{"foo/bar/+/baz"}, "https://example.appspot.com/logs/test/foo/bar/+/baz"},
				{[]types.StreamPath{
					"foo/bar/+/baz",
					"foo/bar/+/qux",
				}, "https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz&s=test%2Ffoo%2Fbar%2F%2B%2Fqux"},
			} {
				Convey(fmt.Sprintf(`Will generate [%s] from %q`, tc.url, tc.paths), func() {
					url, err := bs.GetViewerURL(tc.paths...)
					So(err, ShouldBeNil)
					So(url, ShouldEqual, tc.url)
				})
			}
		})

		Convey(`With no project, will not generate URLs.`, func() {
			bs.Project = ""

			_, err := bs.GetViewerURL("bar")
			So(err, ShouldErrLike, "no project is configured")
		})

		Convey(`With no coordinator host, will not generate URLs.`, func() {
			bs.CoordinatorHost = ""

			_, err := bs.GetViewerURL("bar")
			So(err, ShouldErrLike, "no coordinator host is configured")
		})

		Convey(`With a stream client configured`, func() {
			reg := streamclient.Registry{}
			reg.Register("test", func(spec string, ns types.StreamName) (streamclient.Client, error) {
				return &sentinelClient{}, nil
			})

			So(bs.initializeClient("test:", &reg), ShouldBeNil)
			So(bs.Client, ShouldHaveSameTypeAs, &sentinelClient{})

			Convey(`Can generate viewer URLs for streams.`, func() {
				barS, err := bs.Client.NewStream(streamproto.Flags{Name: "bar"})
				So(err, ShouldBeNil)
				defer barS.Close()

				bazS, err := bs.Client.NewStream(streamproto.Flags{Name: "baz"})
				So(err, ShouldBeNil)
				defer bazS.Close()

				url, err := bs.GetViewerURLForStreams(barS, bazS)
				So(err, ShouldBeNil)
				So(url, ShouldEqual, "https://example.appspot.com/v/?s=test%2Ffoo%2F%2B%2Fbar&s=test%2Ffoo%2F%2B%2Fbaz")
			})

			Convey(`Will not generate viewer URLs if a prefix is not defined.`, func() {
				bs.Prefix = ""

				barS, err := bs.Client.NewStream(streamproto.Flags{Name: "bar"})
				So(err, ShouldBeNil)
				defer barS.Close()

				_, err = bs.GetViewerURLForStreams(barS)
				So(err, ShouldErrLike, "no prefix is configured")
			})
		})
	})
}
