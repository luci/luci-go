// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"testing"

	"github.com/luci/luci-go/client/environ"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamclient"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamproto"
	"github.com/luci/luci-go/logdog/common/types"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
		reg.Register("test", func(spec string) (streamclient.Client, error) {
			regSpec = spec
			return &sentinelClient{}, regErr
		})

		env := environ.Environment{
			"IRRELEVANT": "VALUE",
		}

		Convey(`With no Butler values will return ErrNotBootstrapped.`, func() {
			_, err := getFromEnv(env, reg)
			So(err, ShouldEqual, ErrNotBootstrapped)
		})

		Convey(`With a Butler project and prefix`, func() {
			env[EnvStreamProject] = "test-project"
			env[EnvStreamPrefix] = "butler/prefix"

			Convey(`Yields a Bootstrap with a Project, Prefix, and no Client.`, func() {
				bs, err := getFromEnv(env, reg)
				So(err, ShouldBeNil)

				So(bs, ShouldResemble, &Bootstrap{
					Project: "test-project",
					Prefix:  "butler/prefix",
				})
			})

			Convey(`And the remaining environment parameters`, func() {
				env[EnvStreamServerPath] = "test:client:params"
				env[EnvCoordinatorHost] = "example.appspot.com"

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
				env[EnvStreamPrefix] = "_notavaildprefix"
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate prefix")
			})

			Convey(`With an missing Butler project, will fail.`, func() {
				delete(env, EnvStreamProject)
				_, err := getFromEnv(env, reg)
				So(err, ShouldErrLike, "failed to validate project")
			})

			Convey(`With an invalid Butler project, will fail.`, func() {
				env[EnvStreamProject] = "_notavaildproject"
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
				{[]types.StreamPath{"foo/bar/+/baz"}, "https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz"},
				{[]types.StreamPath{"foo/bar/+/**"}, "https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2F%2A%2A"},
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
			reg.Register("test", func(spec string) (streamclient.Client, error) {
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
