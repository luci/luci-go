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

package grpcutiltest

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/grpc/grpcutil"
)

func TestDetails(t *testing.T) {
	t.Parallel()

	Convey(`Details`, t, func() {
		err := fmt.Errorf("")

		details := []proto.Message{
			&errdetails.DebugInfo{Detail: "1"},
			&errdetails.DebugInfo{Detail: "2"},
		}
		err = grpcutil.WithDetails(err, details...)

		st, serr := grpcutil.ToStatus(err)
		So(serr, ShouldBeNil)
		So(st.Details(), ShouldResembleProto, details)

		Convey(`Second batch`, func() {
			details2 := []proto.Message{
				&errdetails.DebugInfo{Detail: "3"},
				&errdetails.DebugInfo{Detail: "4"},
			}
			err = grpcutil.WithDetails(err, details2...)

			st, serr := grpcutil.ToStatus(err)
			So(serr, ShouldBeNil)
			So(st.Details(), ShouldResembleProto, append(details, details2...))
		})
	})
}
