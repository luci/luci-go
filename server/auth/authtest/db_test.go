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

package authtest

import (
	"context"
	"errors"
	"testing"

	"go.chromium.org/luci/auth/identity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFakeDB(t *testing.T) {
	Convey("FakeDB works", t, func() {
		c := context.Background()
		db := FakeDB{
			"user:abc@def.com": []string{"group a", "group b"},
		}

		resp, err := db.IsMember(c, identity.Identity("user:abc@def.com"), nil)
		So(err, ShouldBeNil)
		So(resp, ShouldBeFalse)

		resp, err = db.IsMember(c, identity.Identity("user:abc@def.com"), []string{"group b"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeTrue)

		resp, err = db.IsMember(c, identity.Identity("user:abc@def.com"), []string{"another", "group b"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeTrue)

		resp, err = db.IsMember(c, identity.Identity("user:another@def.com"), []string{"group b"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeFalse)

		resp, err = db.IsMember(c, identity.Identity("user:another@def.com"), []string{"another", "group b"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeFalse)

		resp, err = db.IsMember(c, identity.Identity("user:abc@def.com"), []string{"another"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeFalse)
	})
}

func TestFakeErroringDB(t *testing.T) {
	Convey("FakeErroringDB works", t, func() {
		c := context.Background()
		db := FakeErroringDB{
			FakeDB: FakeDB{"user:abc@def.com": []string{"group a", "group b"}},
			Error:  errors.New("boo"),
		}

		_, err := db.IsMember(c, identity.Identity("user:abc@def.com"), []string{"group a"})
		So(err.Error(), ShouldEqual, "boo")

		db.Error = nil
		resp, err := db.IsMember(c, identity.Identity("user:abc@def.com"), []string{"group a"})
		So(err, ShouldBeNil)
		So(resp, ShouldBeTrue)
	})
}
