// Copyright 2023 The LUCI Authors.
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

package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRunTask(t *testing.T) {
	t.Parallel()

	Convey("RunTask", t, func() {
		ctx := context.Background()
		srv := &TaskBackendLite{}

		Convey("unimplemented", func() {
			_, err := srv.RunTask(ctx, &pb.RunTaskRequest{})
			So(err, ShouldHaveRPCCode, codes.Unimplemented, "method RunTask not implemented")
		})
	})
}
