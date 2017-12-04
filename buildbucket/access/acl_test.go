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
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/access"

	. "github.com/smartystreets/goconvey/convey"
)

type client struct {
	R   *access.PermittedActionsResponse
	err error
}

// PermittedActions implements the AccessClient interface.
func (c *client) PermittedActions(_ context.Context, req *access.PermittedActionsRequest, _ ...grpc.CallOption) (*access.PermittedActionsResponse, error) {
	return c.R, c.err
}

// Description implements the AccessClient interface.
func (c *client) Description(_ context.Context, _ *empty.Empty, _ ...grpc.CallOption) (*access.DescriptionResponse, error) {
	panic("not implemented")
}

func TestBucketPermissions(t *testing.T) {
	Convey(`A client/server for the Access service`, t, func() {
		c := context.Background()
		client := client{}
		Convey(`Can get sane bucket permissions.`, func() {
			client.R = &access.PermittedActionsResponse{
				Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
					"buck": {
						Actions: []string{
							"ACCESS_BUCKET",
							"VIEW_BUILD",
							"SEARCH_BUILDS",
						},
					},
					"et": {
						Actions: []string{
							"ACCESS_BUCKET",
							"VIEW_BUILD",
							"SEARCH_BUILDS",
							"ADD_BUILD",
							"CANCEL_BUILD",
						},
					},
				},
			}
			perms, err := BucketPermissions(c, &client, []string{"buck", "et"})
			So(err, ShouldBeNil)
			So(perms.Can("buck", AccessBucket), ShouldBeTrue)
			So(perms.Can("buck", ViewBuild), ShouldBeTrue)
			So(perms.Can("buck", SearchBuilds), ShouldBeTrue)
			So(perms.Can("et", AccessBucket), ShouldBeTrue)
			So(perms.Can("et", ViewBuild), ShouldBeTrue)
			So(perms.Can("et", SearchBuilds), ShouldBeTrue)
			So(perms.Can("et", AddBuild), ShouldBeTrue)
			So(perms.Can("et", CancelBuild), ShouldBeTrue)
		})

		Convey(`Can get unknown bucket permissions.`, func() {
			client.R = &access.PermittedActionsResponse{
				Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
					"bucket": {
						Actions: []string{
							"SOMETHING_NEW",
						},
					},
				},
			}
			_, err := BucketPermissions(c, &client, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})

		Convey(`Can deal with error from .`, func() {
			client.err = errors.Reason("haha! you got an error").Err()
			_, err := BucketPermissions(c, &client, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})
	})
}
