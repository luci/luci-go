// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestErrors(t *testing.T) {
	Convey(`Errors`, t, func() {
		Convey(`nil`, func() {
			var err error

			So(AttachStatus(err, bbpb.Status_INFRA_FAILURE, &bbpb.StatusDetails{
				ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
			}), ShouldBeNil)

			status, details := ExtractStatus(err)
			So(status, ShouldResemble, bbpb.Status_SUCCESS)
			So(details, ShouldBeNil)
		})

		Convey(`err`, func() {
			Convey(`generic`, func() {
				err := errors.New("some error")
				status, details := ExtractStatus(err)
				So(status, ShouldResemble, bbpb.Status_FAILURE)
				So(details, ShouldBeNil)
			})

			Convey(`AttachStatus`, func() {
				err := errors.New("some error")
				err2 := AttachStatus(err, bbpb.Status_INFRA_FAILURE, &bbpb.StatusDetails{
					ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
				})

				status, details := ExtractStatus(err2)
				So(status, ShouldResemble, bbpb.Status_INFRA_FAILURE)
				So(details, ShouldResembleProto, &bbpb.StatusDetails{
					ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
				})
			})

			Convey(`context`, func() {
				status, details := ExtractStatus(context.Canceled)
				So(status, ShouldResemble, bbpb.Status_CANCELED)
				So(details, ShouldBeNil)

				status, details = ExtractStatus(context.DeadlineExceeded)
				So(status, ShouldResemble, bbpb.Status_INFRA_FAILURE)
				So(details, ShouldResembleProto, &bbpb.StatusDetails{
					Timeout: &bbpb.StatusDetails_Timeout{},
				})
			})

		})

		Convey(`AttachStatus panics for bad status`, func() {
			So(func() {
				AttachStatus(nil, bbpb.Status_STARTED, nil)
			}, ShouldPanicLike, "cannot be used with non-terminal status \"STARTED\"")
		})
	})
}
