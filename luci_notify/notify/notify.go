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

	"go.chromium.org/gae/service/info"
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
luci-notify detected a status change for builder "{{ .Build.Builder }}"
at {{ .Build.StatusChangeTime }}.

<table>
  <tr>
    <td>New status:</td>
    <td><b>{{ .Build.Status }}</b></td>
  </tr>
  <tr>
    <td>Previous status:</td>
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

// createEmailTask constructs an EmailTask to be dispatched onto the task queue.
func createEmailTask(c context.Context, recipients []string, oldStatus buildbucket.Status, build *Build) (*tq.Task, error) {
	templateContext := map[string]interface{}{
		"OldStatus": oldStatus.String(),
		"Build":     build,
	}
	var bodyBuffer bytes.Buffer
	if err := emailTemplate.Execute(&bodyBuffer, &templateContext); err != nil {
		return nil, errors.Annotate(err, "constructing email body").Err()
	}
	subject := fmt.Sprintf(`[Build Status] Builder %s on %s`,
		build.Builder,
		build.Bucket)

	return &tq.Task{
		Payload: &internal.EmailTask{
			Recipients: recipients,
			Subject:    subject,
			Body:       bodyBuffer.String(),
		},
	}, nil
}

// shouldNotify is the predicate function for whether a trigger's conditions have been met.
func shouldNotify(n *config.NotificationConfig, oldStatus, newStatus buildbucket.Status) bool {
	switch {
	case n.OnSuccess && newStatus == buildbucket.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucket.Status_FAILURE:
	case n.OnChange && oldStatus != StatusUnknown && newStatus != oldStatus:
	default:
		return false
	}
	return true
}

// isRecipientAllowed returns true if the given recipient is allowed to be notified about the given build.
func isRecipientAllowed(c context.Context, recipient string, build *Build) bool {
	// TODO(mknyszek): Do a real ACL check here.
	if strings.HasSuffix(recipient, "@google.com") || strings.HasSuffix(recipient, "@chromium.org") {
		return true
	}
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", build.ID)
	return false
}

// Notify discovers, consolidates and filters recipients from notifiers, and
// 'email_notify' properties, then dispatches notifications if necessary.
func Notify(c context.Context, d *tq.Dispatcher, notifiers []*config.Notifier, oldStatus buildbucket.Status, build *Build) error {
	recipientSet := stringset.New(0)

	// Notify based on configured notifiers.
	for _, n := range notifiers {
		for _, nc := range n.Notifications {
			if !shouldNotify(&nc, oldStatus, build.Status) {
				continue
			}
			for _, r := range nc.EmailRecipients {
				recipientSet.Add(r)
			}
		}
	}

	// Notify based on build request properties.
	for _, r := range build.EmailNotify {
		recipientSet.Add(r)
	}

	for _, r := range recipientSet.ToSlice() {
		if !isRecipientAllowed(c, r, build) {
			recipientSet.Del(r)
		}
	}

	if recipientSet.Len() == 0 {
		logging.Infof(c, "Nobody to notify...")
		return nil
	}
	task, err := createEmailTask(c, recipientSet.ToSlice(), oldStatus, build)
	if err != nil {
		return errors.Annotate(err, "failed to create email task").Err()
	}
	d.AddTask(c, task)
	return nil
}

// InitDispatcher registers the send email task with the given dispatcher.
func InitDispatcher(d *tq.Dispatcher) {
	d.RegisterTask(&internal.EmailTask{}, SendEmail, "email", nil)
}

// SendEmail is a push queue handler that attempts to send an email.
func SendEmail(c context.Context, task proto.Message) error {
	appID := info.AppID(c)
	sender := fmt.Sprintf("%s <noreply@%s.appspotmail.com>", appID, appID)

	// TODO(mknyszek): Query Milo for additional build information.
	emailTask := task.(*internal.EmailTask)
	return mail.Send(c, &mail.Message{
		Sender:   sender,
		To:       emailTask.Recipients,
		Subject:  emailTask.Subject,
		HTMLBody: emailTask.Body,
	})
}
