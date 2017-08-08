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

package memory

import (
	"testing"

	"go.chromium.org/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMustNamespace(t *testing.T) {
	Convey("Testable interface works", t, func() {
		c := UseWithAppID(context.Background(), "dev~app-id")

		// Default value.
		So(info.AppID(c), ShouldEqual, "app-id")
		So(info.FullyQualifiedAppID(c), ShouldEqual, "dev~app-id")
		So(info.RequestID(c), ShouldEqual, "test-request-id")
		sa, err := info.ServiceAccount(c)
		So(err, ShouldBeNil)
		So(sa, ShouldEqual, "gae_service_account@example.com")

		// Setting to "override" applies to initial context.
		c = info.GetTestable(c).SetRequestID("override")
		So(info.RequestID(c), ShouldEqual, "override")

		// Derive inner context, "override" applies.
		c = info.MustNamespace(c, "valid_namespace_name")
		So(info.RequestID(c), ShouldEqual, "override")
	})
}
