// Copyright 2019 The LUCI Authors.
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
package sink

import (
	"context"
	"testing"

	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSinkServer(t *testing.T) {
	t.Parallel()

	Convey("newSinkServer", t, func() {
		ctx := context.Background()
		Convey("succeeds", func() {
			srv, err := newSinkServer(ctx, Options{Address: ":42", AuthToken: "hello"})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
		})
		Convey("creates a random auth token, if not given", func() {
			srv, err := newSinkServer(ctx, Options{Address: ":42"})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.AuthToken, ShouldNotEqual, "")
		})
		Convey("uses the default address, if not given", func() {
			srv, err := newSinkServer(ctx, Options{})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.Address, ShouldNotEqual, "")
		})
	})
}

func TestSinkServerExport(t *testing.T) {
	t.Parallel()

	Convey("Export returns the configured address and auth_token", t, func() {
		ctx := context.Background()
		srv, err := newSinkServer(ctx, Options{Address: "localhost:42", AuthToken: "hello"})
		So(err, ShouldBeNil)
		db := lucictx.GetResultDB(srv.Export(ctx))
		So(db, ShouldNotBeNil)
		So(db.ResultSink, ShouldNotBeNil)
		So(db.ResultSink.Address, ShouldEqual, "localhost:42")
		So(db.ResultSink.AuthToken, ShouldEqual, "hello")
	})
}
