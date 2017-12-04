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

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/common/testing/prpctest"

	"github.com/golang/protobuf/ptypes/empty"
	. "github.com/smartystreets/goconvey/convey"
)

type service struct {
	R   *access.PermittedActionsResponse
	err error
}

func (s *service) PermittedActions(c context.Context, req *access.PermittedActionsRequest) (*access.PermittedActionsResponse, error) {
	return s.R, s.err
}

func (s *service) Description(c context.Context, _ *empty.Empty) (*access.DescriptionResponse, error) {
	panic("not implemented")
}

func TestBucketPermissions(t *testing.T) {
	Convey(`A client/server for the Access service`, t, func() {
		c := context.Background()
		svc := service{}

		// Create a client/server for Greet service.
		ts := prpctest.Server{}
		access.RegisterAccessServer(&ts, &svc)
		ts.Start(c)
		defer ts.Close()

		prpcClient, err := ts.NewClient()
		if err != nil {
			panic(err)
		}
		Convey(`Can get sane bucket permissions.`, func() {
			svc.R = &access.PermittedActionsResponse{
				Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
					"buck": &access.PermittedActionsResponse_ResourcePermissions{
						Actions: []string{
							"ACCESS_BUCKET",
							"VIEW_BUILD",
							"SEARCH_BUILDS",
						},
					},
					"et": &access.PermittedActionsResponse_ResourcePermissions{
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
			perms, err := BucketPermissions(c, prpcClient, []string{"buck", "et"})
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
			svc.R = &access.PermittedActionsResponse{
				Permitted: map[string]*access.PermittedActionsResponse_ResourcePermissions{
					"bucket": &access.PermittedActionsResponse_ResourcePermissions{
						Actions: []string{
							"SOMETHING_NEW",
						},
					},
				},
			}
			_, err := BucketPermissions(c, prpcClient, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})

		Convey(`Can deal with error from .`, func() {
			svc.err = errors.Reason("haha! you got an error").Err()
			_, err := BucketPermissions(c, prpcClient, []string{"bucket"})
			So(err, ShouldNotBeNil)
		})
	})
}
