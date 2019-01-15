// Copyright 2018 The LUCI Authors.
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

package memory

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/gce/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Delete", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("nil", func() {
			cfg, err := srv.Delete(c, nil)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &empty.Empty{})
		})

		Convey("empty", func() {
			cfg, err := srv.Delete(c, &config.DeleteRequest{})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &empty.Empty{})
		})

		Convey("ID", func() {
			cfg, err := srv.Delete(c, &config.DeleteRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &empty.Empty{})
		})

		Convey("deleted", func() {
			srv.cfg.Store("id", &config.Config{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
			cfg, err := srv.Delete(c, &config.DeleteRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &empty.Empty{})
			_, ok := srv.cfg.Load("id")
			So(ok, ShouldBeFalse)
		})
	})

	Convey("Ensure", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("nil", func() {
			cfg, err := srv.Ensure(c, nil)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, (*config.Config)(nil))
			_, ok := srv.cfg.Load("")
			So(ok, ShouldBeTrue)
		})

		Convey("empty", func() {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, (*config.Config)(nil))
			_, ok := srv.cfg.Load("")
			So(ok, ShouldBeTrue)
		})

		Convey("ID", func() {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, (*config.Config)(nil))
			_, ok := srv.cfg.Load("id")
			So(ok, ShouldBeTrue)
		})

		Convey("config", func() {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{
				Id: "id",
				Config: &config.Config{
					Amount: 1,
					Attributes: &config.VM{
						Project: "project",
					},
					Prefix: "prefix",
				},
			})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &config.Config{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
			_, ok := srv.cfg.Load("id")
			So(ok, ShouldBeTrue)
		})
	})

	Convey("Get", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("not found", func() {
			Convey("nil", func() {
				cfg, err := srv.Get(c, nil)
				So(err, ShouldErrLike, "no config found")
				So(cfg, ShouldBeNil)
			})

			Convey("empty", func() {
				cfg, err := srv.Get(c, &config.GetRequest{})
				So(err, ShouldErrLike, "no config found")
				So(cfg, ShouldBeNil)
			})

			Convey("ID", func() {
				cfg, err := srv.Get(c, &config.GetRequest{
					Id: "id",
				})
				So(err, ShouldErrLike, "no config found")
				So(cfg, ShouldBeNil)
			})
		})

		Convey("found", func() {
			srv.cfg.Store("id", &config.Config{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
			cfg, err := srv.Get(c, &config.GetRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &config.Config{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
		})
	})
}
