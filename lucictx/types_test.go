// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lucictx

import (
	"encoding/json"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredefinedTypes(t *testing.T) {
	t.Parallel()

	Convey("Test predefined types", t, func() {
		c := context.Background()
		Convey("local_auth", func() {
			So(GetLocalAuth(c), ShouldBeNil)

			c = SetLocalAuth(c, &LocalAuth{100, []byte("foo")})
			rawJSON := json.RawMessage{}
			Get(c, "local_auth", &rawJSON)
			So(string(rawJSON), ShouldEqual, `{"rpc_port":100,"secret":"Zm9v"}`)

			So(GetLocalAuth(c), ShouldResemble, &LocalAuth{100, []byte("foo")})
		})

		Convey("swarming", func() {
			So(GetSwarming(c), ShouldBeNil)

			c = SetSwarming(c, &Swarming{[]byte("foo")})
			rawJSON := json.RawMessage{}
			Get(c, "swarming", &rawJSON)
			So(string(rawJSON), ShouldEqual, `{"secret_bytes":"Zm9v"}`)

			So(GetSwarming(c), ShouldResemble, &Swarming{[]byte("foo")})
		})
	})
}
