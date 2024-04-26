// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"testing"

	bqpb "go.chromium.org/luci/swarming/proto/bq"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExportToJSON(t *testing.T) {
	t.Parallel()

	Convey("JSON works", t, func() {
		out, err := exportToJSON(&bqpb.BotEvent{
			Bot: &bqpb.Bot{
				BotId: "bot-id",
				Info: &bqpb.BotInfo{
					Supplemental: `{"k1": "v1"}`,
					Version:      "version",
					Host: &bqpb.PhysicalEntity{
						Supplemental: `{"k2": "v2"}`,
					},
					Devices: []*bqpb.PhysicalEntity{
						{Supplemental: `{"k3": "v3"}`},
						{Supplemental: `{"k4": "v4"}`},
					},
				},
			},
			Event:    bqpb.BotEventType_BOT_NEW_SESSION,
			EventMsg: "hello",
		})
		So(err, ShouldBeNil)
		So(out, ShouldEqual, `{
  "bot": {
    "bot_id": "bot-id",
    "info": {
      "devices": [
        {
          "supplemental": {
            "k3": "v3"
          }
        },
        {
          "supplemental": {
            "k4": "v4"
          }
        }
      ],
      "host": {
        "supplemental": {
          "k2": "v2"
        }
      },
      "supplemental": {
        "k1": "v1"
      },
      "version": "version"
    }
  },
  "event": "BOT_NEW_SESSION",
  "event_msg": "hello"
}`)
	})

	Convey("Duration works", t, func() {
		out, err := exportToJSON(&bqpb.TaskRequest{
			TaskSlices: []*bqpb.TaskSlice{
				{
					Properties: &bqpb.TaskProperties{
						ExecutionTimeout: 1.5,
						IoTimeout:        2.5,
						GracePeriod:      3.5,
					},
					Expiration: 4.5,
				},
				{
					Properties: &bqpb.TaskProperties{
						ExecutionTimeout: 5.5,
						IoTimeout:        6.5,
						GracePeriod:      7.5,
					},
					Expiration: 8.5,
				},
			},
			BotPingTolerance: 9.5,
		})
		So(err, ShouldBeNil)
		So(out, ShouldEqual, `{
  "bot_ping_tolerance": "9.500s",
  "task_slices": [
    {
      "expiration": "4.500s",
      "properties": {
        "execution_timeout": "1.500s",
        "grace_period": "3.500s",
        "io_timeout": "2.500s"
      }
    },
    {
      "expiration": "8.500s",
      "properties": {
        "execution_timeout": "5.500s",
        "grace_period": "7.500s",
        "io_timeout": "6.500s"
      }
    }
  ]
}`)
	})
}
