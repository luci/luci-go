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
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	dsS "go.chromium.org/luci/gae/service/datastore"
	infoS "go.chromium.org/luci/gae/service/info"
	mailS "go.chromium.org/luci/gae/service/mail"
	mcS "go.chromium.org/luci/gae/service/memcache"
	modS "go.chromium.org/luci/gae/service/module"
	tqS "go.chromium.org/luci/gae/service/taskqueue"
	userS "go.chromium.org/luci/gae/service/user"
)

func TestContextAccess(t *testing.T) {
	t.Parallel()

	// p is a function which recovers an error and then immediately panics with
	// the contained string which lets us use should.PanicLikeString.
	p := func() { panic(recover().(error).Error()) }

	ftt.Run("Context Access", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("blank", func(t *ftt.Test) {
			assert.Loosely(t, dsS.Raw(c), should.BeNil)
			assert.Loosely(t, mcS.Raw(c), should.BeNil)
			assert.Loosely(t, tqS.Raw(c), should.BeNil)
			assert.Loosely(t, infoS.Raw(c), should.BeNil)
		})

		// needed for everything else
		c = infoS.Set(c, Info())

		t.Run("Info", func(t *ftt.Test) {
			assert.Loosely(t, infoS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				infoS.Raw(c).Datacenter()
			}, should.PanicLikeString("dummy: method Info.Datacenter is not implemented"))

			t.Run("ModuleHostname", func(t *ftt.Test) {
				host, err := infoS.ModuleHostname(c, "", "", "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, host, should.Equal("version.module.dummy-appid.example.com"))

				host, err = infoS.ModuleHostname(c, "wut", "10", "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, host, should.Equal("10.wut.dummy-appid.example.com"))
			})
		})

		t.Run("Datastore", func(t *ftt.Test) {
			c = dsS.SetRaw(c, Datastore())
			assert.Loosely(t, dsS.Raw(c), should.NotBeNil)
			assert.Loosely(t, func() {
				defer p()
				_, _ = dsS.Raw(c).DecodeCursor("wut")
			}, should.PanicLikeString("dummy: method Datastore.DecodeCursor is not implemented"))
		})

		t.Run("Memcache", func(t *ftt.Test) {
			c = mcS.SetRaw(c, Memcache())
			assert.Loosely(t, mcS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				_ = mcS.Add(c, nil)
			}, should.PanicLikeString("dummy: method Memcache.AddMulti is not implemented"))
		})

		t.Run("TaskQueue", func(t *ftt.Test) {
			c = tqS.SetRaw(c, TaskQueue())
			assert.Loosely(t, tqS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				_ = tqS.Purge(c, "")
			}, should.PanicLikeString("dummy: method TaskQueue.Purge is not implemented"))
		})

		t.Run("User", func(t *ftt.Test) {
			c = userS.Set(c, User())
			assert.Loosely(t, userS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				_ = userS.IsAdmin(c)
			}, should.PanicLikeString("dummy: method User.IsAdmin is not implemented"))
		})

		t.Run("Mail", func(t *ftt.Test) {
			c = mailS.Set(c, Mail())
			assert.Loosely(t, mailS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				_ = mailS.Send(c, nil)
			}, should.PanicLikeString("dummy: method Mail.Send is not implemented"))
		})

		t.Run("Module", func(t *ftt.Test) {
			c = modS.Set(c, Module())
			assert.Loosely(t, modS.Raw(c), should.NotBeNilInterface)
			assert.Loosely(t, func() {
				defer p()
				modS.List(c)
			}, should.PanicLikeString("dummy: method Module.List is not implemented"))
		})
	})
}
