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
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/luci/appengine/tq"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/internal"
)

// emailNotify contains information for delivery and personalization of notification emails.
type emailNotify struct {
	Email    string
	Template string
}

// createEmailTasks constructs EmailTasks to be dispatched onto the task queue.
func createEmailTasks(c context.Context, recipients []emailNotify, oldStatus buildbucketpb.Status, build *buildbucketpb.Build) ([]*tq.Task, error) {
	// Get templates.
	bundle, err := getBundle(c, build.Builder.Project)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get a bundle of email templates").Err()
	}

	// Generate emails.
	input := &emailTemplateInput{
		Build:     build,
		OldStatus: oldStatus,
	}
	// An EmailTask with subject and body per template name.
	// They will be used as templates for actual tasks.
	taskTemplates := map[string]*internal.EmailTask{}
	for _, r := range recipients {
		name := r.Template
		if name == "" {
			name = defaultTemplate.Name
		}

		if _, ok := taskTemplates[name]; ok {
			continue
		}

		et := &internal.EmailTask{}
		et.Subject, et.Body = bundle.GenerateEmail(name, input)
		taskTemplates[name] = et
	}

	// Create a task per recipient.
	// Do not bundle multiple recipients into one task because we don't use BCC.
	tasks := make([]*tq.Task, len(recipients))
	for i, r := range recipients {
		name := r.Template
		if name == "" {
			name = defaultTemplate.Name
		}

		task := *taskTemplates[name] // copy
		task.Recipients = []string{r.Email}
		tasks[i] = &tq.Task{
			DeduplicationKey: fmt.Sprintf("%d-%s-%s", build.Id, name, r.Email),
			Payload:          &task,
		}
	}
	return tasks, nil
}


// isRecipientAllowed returns true if the given recipient is allowed to be notified about the given build.
func isRecipientAllowed(c context.Context, recipient string, build *buildbucketpb.Build) bool {
	// TODO(mknyszek): Do a real ACL check here.
	if strings.HasSuffix(recipient, "@google.com") || strings.HasSuffix(recipient, "@chromium.org") {
		return true
	}
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", recipient, build.Id)
	return false
}


// shouldNotify is the predicate function for whether a Notification's trigger conditions have been met.
func shouldNotify(n *buildbucketpb.Notification, oldStatus, newStatus buildbucketpb.Status) bool {
	switch {
	case n.OnSuccess && newStatus == buildbucketpb.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucketpb.Status_FAILURE:
	case n.OnChange && oldStatus != buildbucketpb.Status_STATUS_UNSPECIFIED && newStatus != oldStatus:
	default:
		return false
	}
	return true
}

func extractRecipientsFromBuild(build *buildbucketpb.Build) []emailNotify {
	notifyField, ok := build.Input.Properties.Fields["email_notify"]
	if !ok {
		return nil
	}
	emailNotifyList := notifyField.GetListValue()
	if emailNotifyList == nil {
		return nil
	}
	recipients := make([]emailNotify, 0, len(emailNotifyList))
	for _, value := range emailNotifyList.Values {
		notifyValue := value.GetStructValue()
		if notifyValue == nil {
			continue
		}
		email, ok := notifyValue.Fields["email"]
		if !ok {
			continue
		}
		template, ok := notifyValue.Fields["template"]
		if !ok {
			continue
		}
		recipients = append(recipients, emailNotify{
			Email: email,
			Template: template,
		})
	}
	return recipients
}

func extractRecipientsFromNotifications(notifications notifypb.Notifications, oldStatus, newStatus buildbucketpb.Status) []emailNotify {
	n := notifications.GetNotifications()
	recipients := make([]emailNotify, 0, len(n))
	for _, notification := range n {
		if !shouldNotify(notification, oldStatus, newStatus) {
			continue
		}
		for _, recipient := range notification.GetEmail().GetRecipients() {
			recipients = append(recipients, emailNotify{
				Email:    recipient,
				Template: notification.Template,
			})
		}
	}
	return recipients
}

// Notifier encapsulates the state necessary to construct a set of recipients and send
// notifications. Note that the only field that must be set is Build.
func Notifier struct {
	Build *buildbucketpb.Build
	OldStatus buildbucketpb.Status
	Notifications notifypb.Notifications
	Blamelist []EmailNotify
}

// Notify consolidates the given recipients with those from the 'email_notify' properties,
// filters out unauthorized recipients, then dispatches notifications if necessary.
// Does not dispatch a notification for same email, template and build more than
// once. Ignores current transaction in c, if any.
func (n *Notifier) Notify(c context.Context, d *tq.Dispatcher) error {
	c = datastore.WithoutTransaction(c)

	if n.Build == nil {
		return errors.Reason("no build found to notify about").Err()
	}

	recipients := extractRecipientsFromNotifications(n.Notifications, n.OldStatus, n.Build.Status)
	recipients = append(recipients, extractRecipientsFromBuild(n.Build))
	recipients = append(recipients, n.Blamelist)

	// Remove unallowed recipients.
	allRecipients := recipients
	recipients = recipients[:0]
	for _, r := range allRecipients {
		if isRecipientAllowed(c, r.Email, build) {
			recipients = append(recipients, r)
		}
	}

	if len(recipients) == 0 {
		logging.Infof(c, "Nobody to notify...")
		return nil
	}
	tasks, err := createEmailTasks(c, recipients, n.OldStatus, n.Build)
	if err != nil {
		return errors.Annotate(err, "failed to create email tasks").Err()
	}
	return d.AddTask(c, tasks...)
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
