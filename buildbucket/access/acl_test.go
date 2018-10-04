// Copyright 2017 The LUCI Authors.
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

package access

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/access"

	. "github.com/smartystreets/goconvey/convey"
)

var permittedActionsTestData = &access.PermittedActionsResponse{
	Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
		"buck": {
			Actions: []string{
				"ACCESS_BUCKET",
				"SEARCH_BUILDS",
				"VIEW_BUILD",
			},
		},
		"et": {
			Actions: []string{
				"ACCESS_BUCKET",
				"ADD_BUILD",
				"CANCEL_BUILD",
				"SEARCH_BUILDS",
				"VIEW_BUILD",
			},
		},
	},
	ValidityDuration: &duration.Duration{Seconds: 1},
}

func TestPermissions(t *testing.T) {
	Convey(`Ensure ToProto and FromProto can convert between each other.`, t, func() {
		perms := make(Permissions, 2)
		So(perms.FromProto(permittedActionsTestData), ShouldBeNil)
		So(perms.ToProto(1*time.Second), ShouldResemble, permittedActionsTestData)
	})

	Convey(`Ensure FromProto cleans up Permissions`, t, func() {
		perms := make(Permissions, 2)
		perms["test"] = SearchBuilds
		So(perms.FromProto(permittedActionsTestData), ShouldBeNil)
		So(perms.ToProto(1*time.Second), ShouldResemble, permittedActionsTestData)
	})
}

func TestBucketPermissions(t *testing.T) {
	Convey(`A client/server for the Access service`, t, func() {
		c := context.Background()
		client := TestClient{}
		Convey(`Can get sane bucket permissions.`, func() {
			client.PermittedActionsResponse = permittedActionsTestData
			perms, duration, err := BucketPermissions(c, &client, []string{"buck", "et"})
			So(err, ShouldBeNil)
			So(perms.Can("buck", AccessBucket), ShouldBeTrue)
			So(perms.Can("buck", ViewBuild), ShouldBeTrue)
			So(perms.Can("buck", SearchBuilds), ShouldBeTrue)
			So(perms.Can("et", AccessBucket), ShouldBeTrue)
			So(perms.Can("et", ViewBuild), ShouldBeTrue)
			So(perms.Can("et", SearchBuilds), ShouldBeTrue)
			So(perms.Can("et", AddBuild), ShouldBeTrue)
			So(perms.Can("et", CancelBuild), ShouldBeTrue)
			So(duration, ShouldEqual, 1e9)
		})

		Convey(`Can get unknown bucket permissions.`, func() {
			client.PermittedActionsResponse = &access.PermittedActionsResponse{
				Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
					"bucket": {
						Actions: []string{
							"SOMETHING_NEW",
						},
					},
				},
			}
			_, _, err := BucketPermissions(c, &client, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})

		Convey(`Can deal with error from .`, func() {
			client.Error = errors.Reason("haha! you got an error").Err()
			_, _, err := BucketPermissions(c, &client, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})
	})
}
