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
    builder_defaults {
      category: "quux"
    }
    builders {
      name: "a"
      mixins: "foo_category"
      category: "quuy"
    }
  }
}
`

const buildbucketOut = `buckets: <
  name: "baz"
  swarming: <
    builder_defaults: <
    >
    builders: <
      name: "a"
      mixins: "foo_category"
    >
  >
>
builder_mixins: <
  name: "foo_category"
>
`

func TestBuildbucket(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.BuildbucketCfg{}
		So(proto.UnmarshalText(buildbucketIn, cfg), ShouldBeNil)
		normalizeUnflattenedBuildbucketCfg(cfg)
		So(proto.MarshalTextString(cfg), ShouldEqual, buildbucketOut)
	})
}

const buildbucketNoBuildersIn = `
buckets {
  name: "luci.bar.baz"
}
`

const buildbucketNoBuildersOut = `buckets: <
  name: "baz"
>
`

func TestBuildbucketNoBuilders(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.BuildbucketCfg{}
		So(proto.UnmarshalText(buildbucketNoBuildersIn, cfg), ShouldBeNil)
		normalizeUnflattenedBuildbucketCfg(cfg)
		So(proto.MarshalTextString(cfg), ShouldEqual, buildbucketNoBuildersOut)
	})
}
