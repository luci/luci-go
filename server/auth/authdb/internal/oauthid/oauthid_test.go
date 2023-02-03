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

package oauthid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAllowlist(t *testing.T) {
	Convey("Works", t, func() {
		l := NewAllowlist("primary-client-id", []string{
			"additional-client-id-1",
			"additional-client-id-2",
		})
		call := l.IsAllowedOAuthClientID

		So(call("abc@appspot.gserviceaccount.com", "anonymous"), ShouldBeTrue)
		So(call("dude@example.com", ""), ShouldBeFalse)
		So(call("dude@example.com", GoogleAPIExplorerClientID), ShouldBeTrue)
		So(call("dude@example.com", "primary-client-id"), ShouldBeTrue)
		So(call("dude@example.com", "additional-client-id-2"), ShouldBeTrue)
		So(call("dude@example.com", "unknown-client-id"), ShouldBeFalse)
	})
}
