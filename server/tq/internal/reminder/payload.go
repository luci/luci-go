// Copyright 2020 The LUCI Authors.
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

package reminder

import (
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"google.golang.org/protobuf/proto"
)

// Payload incapsulates the Reminder's payload.
//
// It is produced by Dispatcher and is ultimately consumed by Submitter. It is
// either produced and consumed in the same process (on a "happy path"), or it
// may travel between processes by being stored in a serialized form inside
// a Reminder (as its RawPayload), see Reminder.AttachPayload.
type Payload struct {
	TaskClass string        // corresponding TaskClass.ID, for metrics
	Created   time.Time     // when AddTask was called, for metrics
	Raw       proto.Message // a proto passed to AddTask, available only on happy path

	CreateTaskRequest *taskspb.CreateTaskRequest // prepared Cloud Tasks request
	PublishRequest    *pubsubpb.PublishRequest   // prepared PubSub request
}

// injectReminderID is called when the payload is attached to a reminder.
func (r *Payload) injectReminderID(id string) {
	// We use reminder ID to dedup Cloud Tasks submitted by the sweeper.
	if req := r.CreateTaskRequest; req != nil {
		if req.Task != nil { // may be nil in tests
			req.Task.Name = req.Parent + "/tasks/" + id
		}
	}
	// For PubSub tasks just inject the ID into attributes, for observability.
	if req := r.PublishRequest; req != nil {
		for _, m := range req.Messages {
			m.Attributes["X-Luci-Tq-Reminder-Id"] = id
		}
	}
}
