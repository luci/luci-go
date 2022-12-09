// Copyright 2022 The LUCI Authors.
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

package clients

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidatePubSubTopicName(t *testing.T) {
	Convey("validateTopicName", t, func() {
		Convey("wrong topic name", func() {
			_, _, err := ValidatePubSubTopicName("projects/adsf/")
			So(err, ShouldErrLike, `topic "projects/adsf/" does not match "^projects/(.*)/topics/(.*)$"`)
		})
		Convey("wrong project identifier", func() {
			_, _, err := ValidatePubSubTopicName("projects/pro/topics/topic1")
			So(err, ShouldErrLike, `cloud project id "pro" does not match "^[a-z]([a-z0-9-]){4,28}[a-z0-9]$"`)
		})
		Convey("wrong topic id prefix", func() {
			_, _, err := ValidatePubSubTopicName("projects/cloud-project/topics/goog11")
			So(err, ShouldErrLike, `topic id "goog11" shouldn't begin with the string goog`)
		})
		Convey("wrong topic id format", func() {
			_, _, err := ValidatePubSubTopicName("projects/cloud-project/topics/abc##")
			So(err, ShouldErrLike, `topic id "abc##" does not match "^[A-Za-z]([0-9A-Za-z\\._\\-~+%]){3,255}$"`)
		})
		Convey("success", func() {
			cloudProj, topic, err := ValidatePubSubTopicName("projects/cloud-project/topics/mytopic")
			So(err, ShouldBeNil)
			So(cloudProj, ShouldEqual, "cloud-project")
			So(topic, ShouldEqual, "mytopic")
		})
	})
}
