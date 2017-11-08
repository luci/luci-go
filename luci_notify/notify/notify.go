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
    <td>{{ .OldStatus }}</td>
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

// NotificationSender is a collection of state common to both the recipient aggregation
// and notification dispatch steps of the notifying process. It contains the state and logic
// necessary to construct notifications and dispatch them onto the task queue.
type NotificationSender struct {
	// EmailSender is the email address to send emails from.
	EmailSender string

	// EmailRecipients are the recipients to send emails to.
	EmailRecipients []string

	// OldStatus is the buildbucket status for a builder prior to Build.
	OldStatus buildbucket.Status

	// Build is the most recent buildbucket build recieved for a builder.
	Build *buildbucket.Build
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

// isRecipientAllowed returns true if the given recipient is allowed to be notified about the given build.
func isRecipientAllowed(c context.Context, recipient string, build *buildbucket.Build) bool {
	// TODO(mknyszek): Do a real ACL check here.
	if strings.HasSuffix(recipient, "@google.com") {
		return true
	}
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", build.ID)
	return false
}

// NewSender constructs a new NotificationSender by consolidating and filtering recipients from a list of Notifiers.
func NewSender(c context.Context, emailSender string, notifiers []*config.Notifier, oldStatus buildbucket.Status, build *buildbucket.Build) *NotificationSender {
	recipientSet := stringset.New(0)
	for _, n := range notifiers {
		for _, nc := range n.Notifications {
			if !shouldNotify(&nc, oldStatus, build.Status) {
				continue
			}
			for _, r := range nc.EmailRecipients {
				if isRecipientAllowed(c, r, build) {
					recipientSet.Add(r)
				}
			}
		}

	}
	return &NotificationSender{
		EmailSender:     emailSender,
		EmailRecipients: recipientSet.ToSlice(),
		OldStatus:       oldStatus,
		Build:           build,
	}
}

// createEmailTask constructs an EmailTask to be dispatched onto the task queue.
func (n *NotificationSender) createEmailTask(c context.Context) (*tq.Task, error) {
	templateContext := map[string]interface{}{
		"OldStatus": n.OldStatus.String(),
		"Build":     n.Build,
	}
	var bodyBuffer bytes.Buffer
	if err := emailTemplate.Execute(&bodyBuffer, &templateContext); err != nil {
		return nil, errors.Annotate(err, "constructing email body").Err()
	}
	subject := fmt.Sprintf(`[Build %s] Builder %s on %s`,
		n.Build.Status,
		n.Build.Builder,
		n.Build.Bucket)

	// TODO(mknyszek): Query Milo for additional build information.
	return &tq.Task{
		Payload: &internal.EmailTask{
			Sender:     n.EmailSender,
			Recipients: n.EmailRecipients,
			Subject:    subject,
			Body:       bodyBuffer.String(),
		},
	}, nil
}

// Notify creates an email task and dispatches it onto the task queue.
func (n *NotificationSender) Notify(c context.Context, d *tq.Dispatcher) error {
	if len(n.EmailRecipients) == 0 {
		logging.Infof(c, "Nobody to notify...")
		return nil
	}
	task, err := n.createEmailTask(c)
	if err != nil {
		return errors.Annotate(err, "failed to create email task").Err()
	}
	d.AddTask(c, task)
	return nil
}

// InitDispatcher initializes a dispatcher by registering the SendEmail task.
func InitDispatcher(d *tq.Dispatcher) {
	d.RegisterTask(&internal.EmailTask{}, SendEmail, "email", nil)
}

// SendEmail is a push queue handler that attempts to send an email.
func SendEmail(c context.Context, task proto.Message) error {
	emailTask := task.(*internal.EmailTask)
	return mail.Send(c, &mail.Message{
		Sender:   emailTask.Sender,
		To:       emailTask.Recipients,
		Subject:  emailTask.Subject,
		HTMLBody: emailTask.Body,
	})
}
