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
	pb "go.chromium.org/luci/luci_notify/api/config"
)

const notifyIn = `
notifiers {
  name: "n1"
  notifications {
    on_new_status: FAILURE
    email {
      recipients: "change@google.com"
    }
  }
  builders {
    name: "b2"
    bucket: "b"
  }
  builders {
    name: "b1"
    bucket: "b"
  }
}

notifiers {
  name: "n1"
  notifications {
    on_occurrence: FAILURE
    email {
      recipients: "failure@google.com"
    }
  }
  builders {
    name: "a1"
    bucket: "a"
  }
  builders {
    name: "a2"
    bucket: "a"
  }
}
`

const notifyOut = `notifiers: <
  notifications: <
    on_occurrence: FAILURE
    email: <
      recipients: "failure@google.com"
    >
  >
  builders: <
    bucket: "a"
    name: "a1"
  >
>
notifiers: <
  notifications: <
    on_occurrence: FAILURE
    email: <
      recipients: "failure@google.com"
    >
  >
  builders: <
    bucket: "a"
    name: "a2"
  >
>
notifiers: <
  notifications: <
    on_new_status: FAILURE
    email: <
      recipients: "change@google.com"
    >
  >
  builders: <
    bucket: "b"
    name: "b1"
  >
>
notifiers: <
  notifications: <
    on_new_status: FAILURE
    email: <
      recipients: "change@google.com"
    >
  >
  builders: <
    bucket: "b"
    name: "b2"
  >
>
`

func TestNotify(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		cfg := &pb.ProjectConfig{}
		So(proto.UnmarshalText(notifyIn, cfg), ShouldBeNil)
		So(Notify(context.Background(), cfg), ShouldBeNil)
		So(proto.MarshalTextString(cfg), ShouldEqual, notifyOut)
	})
}
