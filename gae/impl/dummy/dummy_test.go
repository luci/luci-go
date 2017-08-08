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

package dummy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	dsS "go.chromium.org/gae/service/datastore"
	infoS "go.chromium.org/gae/service/info"
	mailS "go.chromium.org/gae/service/mail"
	mcS "go.chromium.org/gae/service/memcache"
	modS "go.chromium.org/gae/service/module"
	tqS "go.chromium.org/gae/service/taskqueue"
	userS "go.chromium.org/gae/service/user"
	"golang.org/x/net/context"
)

func TestContextAccess(t *testing.T) {
	t.Parallel()

	// p is a function which recovers an error and then immediately panics with
	// the contained string. It's defer'd in each test so that we can use the
	// ShouldPanicWith assertion (which does an == comparison and not
	// a reflect.DeepEquals comparison).
	p := func() { panic(recover().(error).Error()) }

	Convey("Context Access", t, func() {
		c := context.Background()

		Convey("blank", func() {
			So(dsS.Raw(c), ShouldBeNil)
			So(mcS.Raw(c), ShouldBeNil)
			So(tqS.Raw(c), ShouldBeNil)
			So(infoS.Raw(c), ShouldBeNil)
		})

		// needed for everything else
		c = infoS.Set(c, Info())

		Convey("Info", func() {
			So(infoS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				infoS.Raw(c).Datacenter()
			}, ShouldPanicWith, "dummy: method Info.Datacenter is not implemented")

			Convey("ModuleHostname", func() {
				host, err := infoS.ModuleHostname(c, "", "", "")
				So(err, ShouldBeNil)
				So(host, ShouldEqual, "version.module.dummy-appid.example.com")

				host, err = infoS.ModuleHostname(c, "wut", "10", "")
				So(err, ShouldBeNil)
				So(host, ShouldEqual, "10.wut.dummy-appid.example.com")
			})
		})

		Convey("Datastore", func() {
			c = dsS.SetRaw(c, Datastore())
			So(dsS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_, _ = dsS.Raw(c).DecodeCursor("wut")
			}, ShouldPanicWith, "dummy: method Datastore.DecodeCursor is not implemented")
		})

		Convey("Memcache", func() {
			c = mcS.SetRaw(c, Memcache())
			So(mcS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = mcS.Add(c, nil)
			}, ShouldPanicWith, "dummy: method Memcache.AddMulti is not implemented")
		})

		Convey("TaskQueue", func() {
			c = tqS.SetRaw(c, TaskQueue())
			So(tqS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = tqS.Purge(c, "")
			}, ShouldPanicWith, "dummy: method TaskQueue.Purge is not implemented")
		})

		Convey("User", func() {
			c = userS.Set(c, User())
			So(userS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = userS.IsAdmin(c)
			}, ShouldPanicWith, "dummy: method User.IsAdmin is not implemented")
		})

		Convey("Mail", func() {
			c = mailS.Set(c, Mail())
			So(mailS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				_ = mailS.Send(c, nil)
			}, ShouldPanicWith, "dummy: method Mail.Send is not implemented")
		})

		Convey("Module", func() {
			c = modS.Set(c, Module())
			So(modS.Raw(c), ShouldNotBeNil)
			So(func() {
				defer p()
				modS.List(c)
			}, ShouldPanicWith, "dummy: method Module.List is not implemented")
		})
	})
}
