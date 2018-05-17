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
	html "html/template"
	"strings"
	text "text/template"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

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

const defaultTemplate = "default"

// createEmailTask constructs an EmailTask to be dispatched onto the task queue.
func createEmailTask(c context.Context, recipients []EmailNotify, oldStatus buildbucketpb.Status, build *Build) ([]*tq.Task, error) {
	templateContext := map[string]interface{}{
		"OldStatus": oldStatus.String(),
		"Build":     build,
	}
	tList := stringset.New(len(recipients))
	for _, rt := range recipients {
		if rt.Template != "" && rt.Template != defaultTemplate {
			tList.Add(rt.Template)
		}
	}
	templates, err := getEmailTemplates(c, tList.ToSlice(), build.Builder.Project)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving email templates").Err()
	}
	if tList.Len()+1 != len(templates) {
		validateTemplates(tList, templates)
	}
	tasks := []*tq.Task{}
	var bodyBuffer bytes.Buffer
	var subjectBuffer bytes.Buffer
	tFuncMap := html.FuncMap{
		"time": func(ts *tspb.Timestamp) time.Time {
			t, _ := ptypes.Timestamp(ts)
			return t
		},
	}
	for name, t := range templates {
		var destEmail []string
		for _, recipient := range recipients {
			if recipient.Template == name || (name == defaultTemplate && recipient.Template == "") {
				destEmail = append(destEmail, recipient.Email)
			}
		}
		if len(destEmail) < 1 {
			continue
		}
		bodyBuffer.Reset()
		subjectBuffer.Reset()
		et := html.New("email").Funcs(tFuncMap)
		if _, err := et.Parse(t.Body); err != nil {
			logging.Infof(c, "parsing email body into template %v: %v", name, err)
			continue
		}
		if err := et.Execute(&bodyBuffer, &templateContext); err != nil {
			logging.Infof(c, "constructing email body for template %v: %v", name, err)
			continue
		}
		es, err := text.New("subject").Parse(strings.TrimSpace(t.Subject))
		if err != nil {
			logging.Infof(c, "parsing subject into template %v: %v", name, err)
		}
		if err := es.Execute(&subjectBuffer, &templateContext); err != nil {
			logging.Infof(c, "constructing email subject for template %v: %v", name, err)
			continue
		}
		for _, rec := range destEmail {
			tasks = append(tasks, &tq.Task{
				Payload: &internal.EmailTask{
					Recipients: []string{rec},
					Subject:    subjectBuffer.String(),
					Body:       bodyBuffer.String(),
				},
			})
		}
	}
	return tasks, nil
}

//validateTemplates compares what was requested and what was retrieved, defaulting templates that have no match to the default overall template.
func validateTemplates(wantedTemplates stringset.Set, retrievedTemplates map[string]subjectAndBody) {
	for _, rt := range wantedTemplates.ToSlice() {
		if _, ok := retrievedTemplates[rt]; !ok {
			retrievedTemplates[rt] = subjectAndBody{
				Subject: defaultSubject,
				Body:    defaultBody,
			}
		}
	}
}

type subjectAndBody struct {
	Subject string
	Body    string
}

// getEmailTemplateMap retrieves all user requested email templates for the provided project.
func getEmailTemplates(c context.Context, templateList []string, project string) (map[string]subjectAndBody, error) {
	lucicfg := notifyConfig.GetConfigService(c)
	templateContent := make([]map[string]subjectAndBody, len(templateList)+1)
	templatePath := info.AppID(c) + "/email_templates/"
	templateMap := make(map[string]subjectAndBody, len(templateList)+1)
	err := parallel.WorkPool(10, func(work chan<- func() error) {
		for i, tf := range templateList {
			work <- func() error {
				i := i
				tf := tf
				cTemplate, err := lucicfg.GetConfig(c, configInterface.ProjectSet(project), fmt.Sprintf("%s%s.template", templatePath, tf), false)
				switch {
				case err == configInterface.ErrNoConfig:
					return errors.Annotate(err, "template does not exist: %s", tf).Err()
				case err != nil:
					return errors.Annotate(err, "while fetching contents for template: %s", tf).Err()
				}
				sb, err := parseTemplate(tf, cTemplate.Content)
				if err != nil {
					return errors.Annotate(err, "while parsing contents for template: %s", tf).Err()
				}
				templateContent[i] = sb
				return nil
			}
		}
	})
	if err != nil {
		return templateMap, errors.Annotate(err, "failed to retrieve email templates").Err()
	}

	// Ensures that default template exists for all projects; overwritten if a default template exists.
	templateMap[defaultTemplate] = subjectAndBody{
		Subject: defaultSubject,
		Body:    defaultBody,
	}
	for _, content := range templateContent {
		for k, v := range content {
			templateMap[k] = v
		}
	}
	return templateMap, nil
}

func parseTemplate(name, content string) (map[string]subjectAndBody, error) {
	parsedContent := make(map[string]subjectAndBody, 1)
	if len(content) == 0 {
		return parsedContent, fmt.Errorf("empty template file, unable to parse subject and body")
	}
	tc := strings.SplitN(content, "\n", 3)
	switch {
	case len(tc) < 3:
		return parsedContent, fmt.Errorf("less than three lines in template file, unable to parse subject and body")
	case strings.TrimSpace(tc[1]) != "":
		return parsedContent, fmt.Errorf("second line is not blank, unable to parse subject and body")
	default:
		parsedContent[name] = subjectAndBody{
			Subject: tc[0],
			Body:    strings.TrimSpace(tc[2]),
		}
	}
	return parsedContent, nil
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
	logging.Warningf(c, "Address %q is not allowed to be notified of build %d", recipient, build.Id)
	return false
}

// Notify discovers, consolidates and filters recipients from notifiers, and
// 'email_notify' properties, then dispatches notifications if necessary.
func Notify(c context.Context, d *tq.Dispatcher, notifiers []*notifyConfig.Notifier, oldStatus buildbucketpb.Status, build *Build) error {
	var recipients []EmailNotify
	// Notify based on configured notifiers.
	for _, n := range notifiers {
		for _, nc := range n.Notifications {
			if !shouldNotify(&nc, oldStatus, build.Status) {
				continue
			}
			for _, r := range nc.EmailRecipients {
				recipients = append(recipients, EmailNotify{
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
	tasks, err := createEmailTask(c, recipients, oldStatus, build)
	if err != nil {
		return errors.Annotate(err, "failed to create email task").Err()
	}
	for _, task := range tasks {
		if err := d.AddTask(c, task); err != nil {
			logging.Warningf(c, "adding task to dispatcher: %v", err)
		}
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
