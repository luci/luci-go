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
	pb "go.chromium.org/luci/milo/api/config"
)

const miloIn = `
headers {
  id: "h"
  tree_status_host: "t1"
}

consoles {
  id: "z"
  header_id: "h"
}

consoles {
  id: "a"
  header {
    tree_status_host: "t2"
  }
  manifest_name: "REVISION"
  refs: "regexp:z"
  refs: "refs/heads/main"
  refs: "y\\z"

  builders {
    name: "zz"
  }
  builders {
    name: "a"
  }
}

consoles {
  id: "b"
  builder_view_only: true
  header_id: "h"
  repo_url: "repo"
  refs: "refs/heads/main"
  builders {
    name: "a"
    category: "cat-a",
    short_name: "sna"
  }
  builders {
    name: "b"
    category: "cat-b",
    short_name: "snb"
  }
}
`

const miloOut = `consoles: <
  id: "a"
  refs: "regexp:refs/heads/main"
  refs: "regexp:y\\\\z"
  refs: "regexp:z"
  builders: <
    name: "zz"
  >
  builders: <
    name: "a"
  >
  header: <
    tree_status_host: "t2"
  >
>
consoles: <
  id: "b"
  builders: <
    name: "a"
  >
  builders: <
    name: "b"
  >
  builder_view_only: true
>
consoles: <
  id: "z"
  header: <
    tree_status_host: "t1"
  >
>
`

func TestMilo(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.Project{}
		So(proto.UnmarshalText(miloIn, cfg), ShouldBeNil)
		So(Milo(context.Background(), cfg), ShouldBeNil)
		So(proto.MarshalTextString(cfg), ShouldEqual, miloOut)
	})
}
