// Copyright 2017 The LUCI Authors.
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

package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/mail"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
)

var emailTemplate = template.Must(template.New("email").Parse(`
luci-notify has detected a new {{ .Build.Status }} on builder "{{ .Build.Builder }}".

<table>
  <tr>
    <td>Previous result:</td>
    <td>{{ .Builder.Status }}</td>
  </tr>
  <tr>
    <td>Bucket:</td>
    <td>{{ .Build.Bucket }}</td>
  </tr>
  <tr>
    <td>Created by:</td>
    <td>{{ .Build.CreatedBy }}</td>
  </tr>
  <tr>
    <td>Created at:</td>
    <td>{{ .Build.CreationTime }}</td>
  </tr>
  <tr>
    <td>Finished at:</td>
    <td>{{ .Build.CompletionTime }}</td>
  </tr>
</table>

<a href="{{ .Build.URL }}">Full details are available here.</a>`))

// Notification represents a notification ready to be sent.
//
// The purpose of Notification is to abstract away the details of sending
// notifications, detecting triggers, and of consolidating recipients to
// minimize the number of notifications sent.
//
// TODO(mknyszek): Support IRC and Webhooks.
type Notification struct {
	// EmailRecipients is a list of email addresses to notify.
	EmailRecipients []string

	// Build is the current build we are notifying about.
	//
	// This is used primarily for constructing the content of the
	// final notification for each communication channel.
	Build *buildbucket.Build

	// Builder is the Builder before Build was seen.
	//
	// This is used primarily for constructing the content of the
	// final notification for each communication channel.
	Builder *Builder
}

// isRecipientAllowed returns true if the given recipient is allowed to be notified about the given build.
func isRecipientAllowed(c context.Context, recipient string, build *buildbucket.Build) bool {
	// TODO(mknyszek): Do a real ACL check here.
	if strings.HasSuffix(recipient, "@google.com") {
		return true
	}
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", build.ID)
	return false
}

// shouldNotify is the predicate function for whether a trigger's conditions have been met.
func shouldNotify(n *config.NotificationConfig, oldStatus, newStatus buildbucket.Status) bool {
	switch {
	case n.OnSuccess && newStatus == buildbucket.StatusSuccess:
	case n.OnFailure && newStatus == buildbucket.StatusFailure:
	case n.OnChange && oldStatus != StatusUnknown && newStatus != oldStatus:
	default:
		return false
	}
	return true
}

// CreateNotification consolidates recipients from a list of Notifiers and produces a Notification.
//
// This function also checks whether the triggers specified in the Notifiers have been met, and
// filters out recipients from the list of Notifiers appropriately. If there are no recipients to
// send to, then no Notification is created.
func CreateNotification(c context.Context, notifiers []*config.Notifier, build *buildbucket.Build, builder *Builder) *Notification {
	recipientSet := stringset.New(0)
	for _, n := range notifiers {
		for _, nc := range n.Notifications {
			if !shouldNotify(&nc, builder.Status, build.Status) {
				continue
			}
			for _, r := range nc.EmailRecipients {
				if isRecipientAllowed(c, r, build) {
					recipientSet.Add(r)
				}
			}
		}

	}
	return &Notification{
		EmailRecipients: recipientSet.ToSlice(),
		Build: build,
		Builder: builder,
	}
}

func (n *Notification) DispatchEmail(c context.Context, d *tq.Dispatcher) error {
	if len(n.EmailRecipients) == 0 {
		return nil
	}
	var bodyBuffer bytes.Buffer
	if err := emailTemplate.Execute(&bodyBuffer, n); err != nil {
		return errors.Annotate(err, "constructing email body").Err()
	}
	subject := fmt.Sprintf(`[Build %s] Builder %s on %s`,
		n.Build.Status,
		n.Build.Builder,
		n.Build.Bucket)

	d.AddTask(c, &tq.Task{
		Payload: &internal.EmailTask{
			Recipients: n.EmailRecipients,
			Subject:    subject,
			BuildInfo:  bodyBuffer.String(),
			Milo: &internal.MiloInfo{
				SwarmingTask: n.Build.Tags.Get("swarming_task_id"),
			},
		},
	})
	return nil
}

// SendEmail is a push queue handler that attempts to send an email.
func SendEmail(c context.Context, task proto.Message) error {
	// TODO(mknyszek): Query Milo for additional build information.
	emailTask := task.(*internal.EmailTask)
	return mail.Send(c, &mail.Message{
		Sender:   "luci-notify <noreply@luci-notify-dev.appspotmail.com>",
		To:       emailTask.Recipients,
		Subject:  emailTask.Subject,
		HTMLBody: emailTask.BuildInfo,
	})
}
