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
	"errors"
	"net"
	"testing"

	"github.com/luci/luci-go/server/auth"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFakeState(t *testing.T) {
	Convey("Default FakeState works", t, func() {
		state := FakeState{}
		So(state.DB(), ShouldResemble, &FakeErroringDB{FakeDB: FakeDB{"anonymous:anonymous": nil}})
		So(state.Method(), ShouldNotBeNil)
		So(state.User(), ShouldResemble, &auth.User{Identity: "anonymous:anonymous"})
		So(state.PeerIdentity(), ShouldEqual, "anonymous:anonymous")
		So(state.PeerIP().String(), ShouldEqual, "127.0.0.1")
	})

	Convey("Non-default FakeState works", t, func() {
		state := FakeState{
			Identity:             "user:abc@def.com",
			IdentityGroups:       []string{"abc"},
			Error:                errors.New("boo"),
			PeerIdentityOverride: "bot:blah",
			PeerIPOverride:       net.ParseIP("192.192.192.192"),
		}
		So(state.DB(), ShouldResemble, &FakeErroringDB{
			FakeDB: FakeDB{"user:abc@def.com": []string{"abc"}},
			Error:  state.Error,
		})
		So(state.Method(), ShouldNotBeNil)
		So(state.User(), ShouldResemble, &auth.User{
			Identity: "user:abc@def.com",
			Email:    "abc@def.com",
		})
		So(state.PeerIdentity(), ShouldEqual, "bot:blah")
		So(state.PeerIP().String(), ShouldEqual, "192.192.192.192")
	})
}
