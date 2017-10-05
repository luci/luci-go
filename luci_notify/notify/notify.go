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

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/mail"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/luci_notify/config"
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

// sendEmail constructs and sends an email built from this notification.
func sendEmail(c context.Context, recipients []string, build *buildbucket.Build, builder *Builder) error {
	templateContext := map[string]interface{}{
		"Build":   build,
		"Builder": builder,
	}
	var bodyBuffer bytes.Buffer
	if err := emailTemplate.Execute(&bodyBuffer, &templateContext); err != nil {
		return errors.Annotate(err, "constructing email body").Err()
	}
	subject := fmt.Sprintf(`[Build %s] Builder %s on %s`,
		build.Status,
		build.Builder,
		build.Bucket)

	return mail.Send(c, &mail.Message{
		Sender:   "luci-notify <noreply@luci-notify-dev.appspotmail.com>",
		To:       recipients,
		Subject:  subject,
		HTMLBody: bodyBuffer.String(),
	})
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

// Notify consolidates and filters recipients from a list of Notifiers and dispatches
// notifications if necessary.
func Notify(c context.Context, notifiers []*config.Notifier, build *buildbucket.Build, builder *Builder) error {
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
	if recipientSet.Len() == 0 {
		logging.Infof(c, "Nobody to notify...")
		return nil
	}
	return errors.Annotate(sendEmail(c, recipientSet.ToSlice(), build, builder), "failed to send email").Tag(transient.Tag).Err()
}
