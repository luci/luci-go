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
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	configInterface "go.chromium.org/luci/config"
	notifyConfig "go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
)

var (
	defaultBody = `luci-notify detected a status change for builder "{{ .Build.Builder.IDString }}"
at {{ .Build.EndTime | time }}.

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
    <td>Builder:</td>
    <td>{{ .Build.Builder.IDString }}</td>
  </tr>
  <tr>
    <td>Created by:</td>
    <td>{{ .Build.CreatedBy }}</td>
  </tr>
  <tr>
    <td>Created at:</td>
    <td>{{ .Build.CreateTime | time }}</td>
  </tr>
  <tr>
    <td>Finished at:</td>
    <td>{{ .Build.EndTime | time }}</td>
  </tr>
</table>

<a href="{{ .Build.ViewUrl }}">Full details are available here.</a><br/><br/>

You are receiving the default template as no template was provided or a template
name did not match the one provided.`

	defaultSubject = `[Build Status] Builder "{{ .Build.Builder.IDString }}"`
)

// createEmailTask constructs an EmailTask to be dispatched onto the task queue.
func createEmailTask(c context.Context, recipients []emailNotify, oldStatus buildbucketpb.Status, build *Build) ([]*tq.Task, error) {
	templateContext := map[string]interface{}{
		"OldStatus": oldStatus.String(),
		"Build":     build.BuildBucket,
	}
	tasks := []*tq.Task{}
	templates, err := emailTemplates(c, build)
	if err != nil {
		return tasks, errors.Annotate(err, "retrieving email template").Err()
	}
	// TODO(mikenichols): Provide reporting metrics on requested templates not existing.
	for _, recipient := range recipients {
		emailBody := defaultBody
		emailSubject := defaultSubject
		var bodyBuffer bytes.Buffer
		for _, t := range templates {
			if recipient.Template == t.Template {
				emailBody = (t.Body)
				emailSubject = t.Subject
			}
		}
		et := template.Must(template.New("email").Funcs(template.FuncMap{
			"time": func(ts *tspb.Timestamp) time.Time {
				t, _ := ptypes.Timestamp(ts)
				return t
			},
		}).Parse(defaultBody))
		if err := et.Execute(&bodyBuffer, &templateContext); err != nil {
			return nil, errors.Annotate(err, "constructing email body").Err()
		}
		tasks = append(tasks, &tq.Task{
			Payload: &internal.EmailTask{
				Recipients: []string{recipient.Email},
				Subject:    emailSubject,
				Body:       emailBody,
			},
		})
	}
	return tasks, nil
}

type emailMap struct {
	Template string
	Subject  string
	Body     string
}

// emailTemplates provided template name with templates files associated with project.
func emailTemplates(c context.Context, build *Build) ([]emailMap, error) {
	lucicfg := c.Value("configInterface").(configInterface.Interface)
	files, err := lucicfg.ListFiles(c, configInterface.ProjectSet(build.BuildBucket.Builder.Project))
	if err != nil {
		return nil, errors.Annotate(err, "while fetching project file list").Err()
	}
	templateMap := []emailMap{}
	for _, path := range files {
		cTemplate, err := lucicfg.GetConfig(c, configInterface.ProjectSet(build.BuildBucket.Builder.Project), path, false)
		if err != nil {
			return templateMap, errors.Annotate(err, "while fetching template contents").Err()
		}
		var tp []string
		if strings.Contains(path, "/") {
			tp = strings.Split(path, "/")
		} else {
			tp = append(tp, path)
		}
		tn := strings.Split(tp[len(tp)-1], ".template")
		if len(cTemplate.Content) > 0 && len(tp) > 1 {
			tc := strings.Split(cTemplate.Content, "\n")
			if len(tc) > 1 {
				templateMap = append(templateMap, emailMap{
					Template: tn[0],
					Subject:  tc[0],
					Body:     tc[1],
				})
			}
		}
	}
	return templateMap, nil
}

// shouldNotify is the predicate function for whether a trigger's conditions have been met.
func shouldNotify(n *notifyConfig.NotificationConfig, oldStatus, newStatus buildbucketpb.Status) bool {
	switch {
	case n.OnSuccess && newStatus == buildbucketpb.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucketpb.Status_FAILURE:
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
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", recipient, build.BuildBucket.Id)
	return false
}

// Notify discovers, consolidates and filters recipients from notifiers, and
// 'email_notify' properties, then dispatches notifications if necessary.
func Notify(c context.Context, d *tq.Dispatcher, notifiers []*notifyConfig.Notifier, oldStatus buildbucketpb.Status, build *Build) error {
	var recipients []emailNotify
	// Notify based on configured notifiers.
	for _, n := range notifiers {
		for _, nc := range n.Notifications {
			if !shouldNotify(&nc, oldStatus, build.BuildBucket.Status) {
				continue
			}
			for _, r := range nc.EmailRecipients {
				recipients = append(recipients, emailNotify{
					Email:    r,
					Template: nc.Template,
				})
			}
		}
	}

	// Notify based on build request properties.
	recipients = append(recipients, build.EmailNotify...)

	for i, r := range recipients {
		if !isRecipientAllowed(c, r.Email, build) {
			recipients = append(recipients[:i], recipients[i+1:]...)
		}
	}

	if len(recipients) == 0 {
		logging.Infof(c, "Nobody to notify...")
		return nil
	}
	c = notifyConfig.NotifyInterface(c)
	tasks, err := createEmailTask(c, recipients, oldStatus, build)
	if err != nil {
		return errors.Annotate(err, "failed to create email task").Err()
	}
	for _, task := range tasks {
		d.AddTask(c, task)
	}
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
