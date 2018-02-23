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

package testconfig

import (
	"net/url"
	"testing"

	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func tpb(msg proto.Message) string { return proto.MarshalTextString(msg) }

func accessCfg(access ...string) string {
	return tpb(&configPB.ProjectCfg{
		Access: access,
	})
}

func TestLocalService(t *testing.T) {
	t.Parallel()

	Convey(`Testing the local service`, t, func() {
		c := context.Background()

		fs := authtest.FakeState{
			Identity:       "user:foo@bar.baz",
			IdentityGroups: []string{"all", "special"},
		}
		c = auth.WithState(c, &fs)

		configs := map[string]memory.ConfigSet{
			"projects/foo": {
				"path.cfg":    "foo",
				"project.cfg": accessCfg("group:all"),
			},
			"projects/exclusive": {
				"path.cfg":    "exclusive",
				"project.cfg": accessCfg("group:special"),
			},
			"projects/exclusive/refs/heads/master": {
				"path.cfg": "exclusive master",
			},
			"projects/nouser": {
				"path.cfg":    "nouser",
				"project.cfg": accessCfg("group:impossible"),
			},
			"projects/nouser/refs/heads/master": {
				"path.cfg": "nouser master",
			},
			"services/baz": {
				"path.cfg": "service only",
			},
		}
		mbase := memory.New(configs)
		c = backend.WithBackend(c, &client.Backend{&Provider{
			Base: mbase,
		}})

		metaFor := func(configSet, path string) *cfgclient.Meta {
			cfg, err := mbase.GetConfig(c, configSet, path, false)
			if err != nil {
				panic(err)
			}
			return &cfgclient.Meta{
				ConfigSet:   config.Set(cfg.ConfigSet),
				Path:        cfg.Path,
				ContentHash: cfg.ContentHash,
				Revision:    cfg.Revision,
				ViewURL:     cfg.ViewURL,
			}
		}

		Convey(`Can get the service URL`, func() {
			So(cfgclient.ServiceURL(c), ShouldResemble, url.URL{Scheme: "test", Host: "example.com"})
		})

		Convey(`Can get a single config`, func() {
			var (
				val  string
				meta cfgclient.Meta
			)

			Convey(`AsService`, func() {
				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "path.cfg", cfgclient.String(&val), &meta), ShouldBeNil)
				So(val, ShouldEqual, "foo")
				So(&meta, ShouldResemble, metaFor("projects/foo", "path.cfg"))

				So(cfgclient.Get(c, cfgclient.AsService, "projects/exclusive", "path.cfg", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "exclusive")

				So(cfgclient.Get(c, cfgclient.AsService, "services/baz", "path.cfg", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "service only")
			})

			Convey(`AsUser`, func() {
				So(cfgclient.Get(c, cfgclient.AsUser, "projects/foo", "path.cfg", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "foo")

				So(cfgclient.Get(c, cfgclient.AsUser, "projects/exclusive", "path.cfg", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "exclusive")

				So(cfgclient.Get(c, cfgclient.AsUser, "services/baz", "path.cfg", cfgclient.String(&val), nil),
					ShouldEqual, cfgclient.ErrNoConfig)
			})

			Convey(`AsAnonymous`, func() {
				fs.IdentityGroups = []string{"all"}

				So(cfgclient.Get(c, cfgclient.AsAnonymous, "projects/foo", "path.cfg", cfgclient.String(&val), nil), ShouldBeNil)
				So(val, ShouldEqual, "foo")

				So(cfgclient.Get(c, cfgclient.AsAnonymous, "projects/exclusive", "path.cfg", cfgclient.String(&val), nil),
					ShouldEqual, cfgclient.ErrNoConfig)
				So(cfgclient.Get(c, cfgclient.AsAnonymous, "services/baz", "path.cfg", cfgclient.String(&val), nil),
					ShouldEqual, cfgclient.ErrNoConfig)
			})
		})

		Convey(`Can get multiple configs`, func() {
			var vals []string
			var meta []*cfgclient.Meta

			Convey(`AsService`, func() {
				So(cfgclient.Projects(c, cfgclient.AsService, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldResemble, []string{"exclusive", "foo", "nouser"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/exclusive", "path.cfg"),
					metaFor("projects/foo", "path.cfg"),
					metaFor("projects/nouser", "path.cfg"),
				})

				So(cfgclient.Refs(c, cfgclient.AsService, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldResemble, []string{"exclusive master", "nouser master"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/exclusive/refs/heads/master", "path.cfg"),
					metaFor("projects/nouser/refs/heads/master", "path.cfg"),
				})
			})

			Convey(`AsUser`, func() {
				So(cfgclient.Projects(c, cfgclient.AsUser, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldResemble, []string{"exclusive", "foo"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/exclusive", "path.cfg"),
					metaFor("projects/foo", "path.cfg"),
				})

				So(cfgclient.Refs(c, cfgclient.AsUser, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldResemble, []string{"exclusive master"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/exclusive/refs/heads/master", "path.cfg"),
				})
			})

			Convey(`AsAnonymous`, func() {
				fs.IdentityGroups = []string{"all"}

				So(cfgclient.Projects(c, cfgclient.AsAnonymous, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldResemble, []string{"foo"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/foo", "path.cfg"),
				})

				So(cfgclient.Refs(c, cfgclient.AsAnonymous, "path.cfg", cfgclient.StringSlice(&vals), &meta),
					ShouldBeNil)
				So(vals, ShouldHaveLength, 0)
				So(meta, ShouldHaveLength, 0)
			})
		})
	})
}
