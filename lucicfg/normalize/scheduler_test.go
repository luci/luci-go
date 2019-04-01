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
	pb "go.chromium.org/luci/scheduler/appengine/messages"
)

const schedulerIn = `
acl_sets {
  name: "acls 1"
  acls {
    role: READER
    granted_to: "group:all"
  }
}

acl_sets {
  name: "acls 2"
  acls {
    role: OWNER
    granted_to: "owner@example.com"
  }
}

job {
  id: "z"
  acl_sets: "acls 1"
  acl_sets: "acls 2"
  acls {
    role: READER
    granted_to: "reader@example.com"
  }
  acls {
    role: OWNER
    granted_to: "user:owner@example.com"
  }
}

job {
  id: "a"
}

trigger {
  id: "z"
}

trigger {
  id: "a"
  gitiles: {
    refs: "regexp:z"
    refs: "refs/heads/master"
    refs: "y\\z"
  }
}
`

const schedulerOut = `job: <
  id: "a"
>
job: <
  id: "z"
  acls: <
    granted_to: "group:all"
  >
  acls: <
    granted_to: "user:reader@example.com"
  >
  acls: <
    role: OWNER
    granted_to: "user:owner@example.com"
  >
>
trigger: <
  id: "a"
  gitiles: <
    refs: "regexp:refs/heads/master"
    refs: "regexp:y\\\\z"
    refs: "regexp:z"
  >
>
trigger: <
  id: "z"
>
`

func TestScheduler(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.ProjectConfig{}
		So(proto.UnmarshalText(schedulerIn, cfg), ShouldBeNil)
		So(Scheduler(context.Background(), cfg), ShouldBeNil)
		So(proto.MarshalTextString(cfg), ShouldEqual, schedulerOut)
	})
}
