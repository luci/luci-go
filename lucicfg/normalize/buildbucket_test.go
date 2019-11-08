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

package normalize

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"
	pb "go.chromium.org/luci/buildbucket/proto"
)

const buildbucketIn = `builder_mixins {
  name: "foo_category"
  category: "foo"
}

buckets {
  name: "luci.bar.baz"

  swarming {
    url_format: "url-format"
    builders {
      name: "a"
      mixins: "foo_category"
    }
  }
}
`

const buildbucketOut = `buckets: <
  name: "baz"
  swarming: <
    builders: <
      name: "a"
      dimensions: "pool:baz"
    >
  >
>
`

func TestBuildbucket(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.BuildbucketCfg{}
		So(proto.UnmarshalText(buildbucketIn, cfg), ShouldBeNil)
		So(Buildbucket(context.Background(), cfg), ShouldBeNil)
		So(proto.MarshalTextString(cfg), ShouldEqual, buildbucketOut)
	})
}
