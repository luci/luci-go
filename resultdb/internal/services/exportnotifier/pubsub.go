// Copyright 2022 The LUCI Authors.
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

package exportnotifier

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	v1NotifyReadyForExportTaskClass = "v1-publish-invocation-ready-for-export"
	v1NotifyReadyForExportTopic     = "v1.invocation_ready_for_export"
)

// NotifyReadyForExportPublisher describes how to publish to cloud pub/sub
// notifications that an invocation has been finalized.
var NotifyReadyForExportPublisher = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "notify-ready-for-export",
	Topic:     v1NotifyReadyForExportTopic,
	Prototype: &taskspb.NotificationInvocationReadyForExport{},
	Kind:      tq.Transactional,
	Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
		// Custom serialisation handler needed to control
		// how the message is sent, as the backend is
		// Cloud Pub/Sub and not Cloud Tasks.
		t := m.(*taskspb.NotificationInvocationReadyForExport)
		notification := t.Message
		blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(notification)
		if err != nil {
			return nil, err
		}

		// Prepare attributes, which are can be used by subscribers to
		// filter the messages they receive.
		if err := realms.ValidateRealmName(notification.RootInvocationRealm, realms.GlobalScope); err != nil {
			return nil, err
		}
		project, _ := realms.Split(notification.RootInvocationRealm)
		attrs := map[string]string{
			"luci_project": project,
		}

		return &tq.CustomPayload{
			Meta: attrs,
			Body: blob,
		}, nil
	},
})

// notifyInvocationReadyForExport transactionally enqueues
// a task to publish that the given invocation is ready for export.
func notifyInvocationReadyForExport(ctx context.Context, message *pb.InvocationReadyForExportNotification) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.NotificationInvocationReadyForExport{Message: message},
	})
}
