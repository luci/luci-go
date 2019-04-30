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

package luciexe

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadBuildSecrets(t *testing.T) {
	t.Parallel()

	Convey("readBuildSecrets", t, func() {
		ctx := context.Background()
		ctx = lucictx.SetSwarming(ctx, nil)

		Convey("empty", func() {
			secrets, err := readBuildSecrets(ctx)
			So(err, ShouldBeNil)
			So(secrets, ShouldBeNil)
		})

		Convey("build token", func() {
			secretBytes, err := proto.Marshal(&pb.BuildSecrets{
				BuildToken: "build token",
			})
			So(err, ShouldBeNil)

			ctx = lucictx.SetSwarming(ctx, &lucictx.Swarming{SecretBytes: secretBytes})

			secrets, err := readBuildSecrets(ctx)
			So(err, ShouldBeNil)
			So(string(secrets.BuildToken), ShouldEqual, "build token")
		})
	})
}
