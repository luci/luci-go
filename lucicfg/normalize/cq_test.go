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
	pb "go.chromium.org/luci/cq/api/config/v2"
)

const cqIn = `
config_groups {
  gerrit {
    url: "z"
    projects {
      name: "z"
    }
    projects {
      name: "a"
      ref_regexp: "z"
      ref_regexp: "a"
    }
  }

  gerrit {
    url: "y"
  }

  verifiers {
    gerrit_cq_ability {
      committer_list: "z"
      committer_list: "a"
      dry_run_access_list: "z"
      dry_run_access_list: "a"
    }
  }
}

config_groups {
  gerrit {
    url: "a"
    projects {
      name: "z"
    }
  }

  verifiers {
    tryjob {
      builders {
        name: "z"
        location_regexp: "+z"
        location_regexp: "+a"
        location_regexp_exclude: "+z"
        location_regexp_exclude: "+a"
      }
      builders {
        name: "a"
      }
    }
  }
}
`

const cqOut = `config_groups: <
  gerrit: <
    url: "a"
    projects: <
      name: "z"
      ref_regexp: "refs/heads/master"
    >
  >
  verifiers: <
    tryjob: <
      builders: <
        name: "a"
      >
      builders: <
        name: "z"
        location_regexp: "+a"
        location_regexp: "+z"
        location_regexp_exclude: "+a"
        location_regexp_exclude: "+z"
      >
      retry_config: <
      >
    >
  >
>
config_groups: <
  gerrit: <
    url: "y"
  >
  gerrit: <
    url: "z"
    projects: <
      name: "a"
      ref_regexp: "a"
      ref_regexp: "z"
    >
    projects: <
      name: "z"
      ref_regexp: "refs/heads/master"
    >
  >
  verifiers: <
    gerrit_cq_ability: <
      committer_list: "a"
      committer_list: "z"
      dry_run_access_list: "a"
      dry_run_access_list: "z"
    >
  >
>
`

func TestCQ(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.Config{}
		So(proto.UnmarshalText(cqIn, cfg), ShouldBeNil)
		So(CQ(context.Background(), cfg), ShouldBeNil)
		So(proto.MarshalTextString(cfg), ShouldEqual, cqOut)
	})
}
