// Copyright 2025 The LUCI Authors.
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
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
)

const (
	BuildBucketHost = "cr-buildbucket.appspot.com"
)

var bhnValidRecipents = []string{"@chromium.org", "@google.com", "@rotations.google.com"}

func NotifyOwners(c context.Context) error {
	c, cancel := context.WithTimeout(c, time.Minute)
	defer cancel()

	configs, err := config.FetchProjects(c)
	if err != nil {
		return err
	}
	logging.Infof(c, "got %d project configs to notify owners of builder health", len(configs))

	// Update each project concurrently.
	err = parallel.WorkPool(10, func(work chan<- func() error) {
		for projectID, project := range configs {
			work <- func() error {
				err := notifyOwner(c, project.BuilderHealthNotifier, projectID)
				if err != nil {
					return errors.Fmt("notifying owners for project %q: %w", projectID, err)
				}
				return nil
			}
		}
	})
	return err
}

func notifyOwner(c context.Context, bhn []*notifypb.BuilderHealthNotifier, projectID string) error {
	// Initialize buildbucket client
	t, err := auth.GetRPCTransport(c, auth.AsProject, auth.WithProject(projectID))
	if err != nil {
		return err
	}
	bbclient := buildbucketpb.NewBuildersClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    BuildBucketHost,
		Options: prpc.DefaultOptions(),
	})
	tasks, err := getNotifyOwnersTasks(c, bhn, bbclient, projectID)
	if err != nil {
		return err
	}
	err = addNotifyOwnerTasksToQueue(c, tasks)
	if err != nil {
		return err
	}
	return nil
}

func addNotifyOwnerTasksToQueue(c context.Context, tasks map[string]*internal.EmailTask) error {
	for emailKey, task := range tasks {
		logging.Debugf(c, "Adding tq email task for %s", emailKey)
		if err := tq.AddTask(c, &tq.Task{
			Payload:          task,
			Title:            emailKey,
			DeduplicationKey: emailKey,
		}); err != nil {
			return err
		}
	}
	return nil
}

func generateEmail(
	c context.Context,
	ownerEmail string,
	unhealthyCount int,
	totalBuilders int,
	builderDescriptionsHTML string,
	healthDocLink string,
) []byte {
	htmlBody := fmt.Sprintf(`
	<html>
	<head>
		<meta charset="utf-8">
	</head>
	<body>
		<p>Hello,</p>
		<p>You are receiving this because <strong>%[1]s</strong> is subscribed to builder health notifier. <strong>%[2]d</strong> of your <strong>%[3]d</strong> builders are in bad health.</p>

		%[4]s

		<p>For more information on builder health, please see the <a href="%[5]s">Builder Health Documentation</a>.</p>
	</body>
	</html>
	`,
		ownerEmail,
		unhealthyCount,
		totalBuilders,
		builderDescriptionsHTML,
		healthDocLink,
	)
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	_, err := io.WriteString(gz, htmlBody)
	if err != nil {
		logging.Debugf(c,"Failed to write string in generateEmail for %s, err is %s", htmlBody, err)
	}
	if err := gz.Close(); err != nil {
		logging.Debugf(c, "Failed to close gzip.NewWriter in generateEmail for %s, err is %s", htmlBody, err)
	}
	logging.Debugf(c, "Completed generating email for %s", ownerEmail)

	return buf.Bytes()
}

// BuilderInfo represents a single builder in the health report.
type BuilderInfo struct {
	Name        string
	Link        string
	Description string
}

func generateBuilderDescriptionHTML(unhealthyBuilders []BuilderInfo, healthyBuilders []BuilderInfo, unknownHealthBuilders []BuilderInfo) string {
	htmlOutput := ""
	if len(unhealthyBuilders) != 0 {
		htmlOutput += "<p><strong>Unhealthy Builders:</strong></p>" + formatBuildersToHTML(unhealthyBuilders)

	}
	if len(healthyBuilders) != 0 {
		htmlOutput += "<p><strong>Healthy Builders:</strong></p>" + formatBuildersToHTML(healthyBuilders)

	}
	if len(unknownHealthBuilders) != 0 {
		htmlOutput += "<p><strong>Unknown Health Builders:</strong></p>" + formatBuildersToHTML(unknownHealthBuilders)

	}
	return htmlOutput
}

func formatBuildersToHTML(builders []BuilderInfo) string {
	if len(builders) == 0 {
		return ""
	}
	htmlOutput := "<ul>" // Start the unordered list
	for _, builder := range builders {
		htmlOutput += "<li>" // Start list item

		// Add builder name and link
		htmlOutput += fmt.Sprintf(`<strong><a href="%s">%s</a></strong>`, builder.Link, builder.Name)

		if builder.Description != "" {
			for _, description := range strings.Split(builder.Description, ";") {
				htmlOutput += fmt.Sprintf(`<p style="margin-left:30px;">%s</p>`, description)
			}
		}
		htmlOutput += "</li>" // End list item
	}
	htmlOutput += "</ul>" // End the unordered list

	return htmlOutput
}

// shouldIgnoreRecipient returns true if the given email recipient is not safe to send to.
func shouldIgnoreRecipient(c context.Context, email string) bool {
	for _, suffix := range validRecipientSuffixes {
		if strings.HasSuffix(email, suffix) {
			return false
		}
	}
	logging.Warningf(c, "Email %q is not allowed to be notified", email)
	return true
}

func getNotifyOwnersTasks(c context.Context, bhn []*notifypb.BuilderHealthNotifier, bbclient buildbucketpb.BuildersClient, project string) (map[string]*internal.EmailTask, error) {
	tasks := make(map[string]*internal.EmailTask)
	for _, builderHealthNotifier := range bhn {
		email := builderHealthNotifier.OwnerEmail
		if shouldIgnoreRecipient(c, email) {
			continue
		}
		healthyBuilders := []BuilderInfo{}
		unhealthyBuilders := []BuilderInfo{}
		unknownHealthBuilders := []BuilderInfo{}
		builderCount := len(builderHealthNotifier.Builders)
		for _, builder := range builderHealthNotifier.Builders {
			req := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: project,
					Bucket:  builder.Bucket,
					Builder: builder.Name,
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			builderItem, err := bbclient.GetBuilder(c, req)
			if err != nil {
				return nil, err
			}
			// Check if metadata or health exists
			// If not, add into unknown category
			if builderItem.Metadata == nil || builderItem.Metadata.Health == nil {
				unknownHealthBuilders = append(unknownHealthBuilders,
					BuilderInfo{
						Name: fmt.Sprintf("%s.%s:%s", project, builder.Bucket, builder.Name),
						Link: fmt.Sprintf("https://ci.chromium.org/ui/p/%s/builders/%s/%s", project, builder.Bucket, builder.Name),
					})
				continue
			}
			healthStatus := builderItem.Metadata.Health
			if healthStatus.HealthScore == 10 {
				healthyBuilders = append(healthyBuilders,
					BuilderInfo{
						Name:        fmt.Sprintf("%s.%s:%s", project, builder.Bucket, builder.Name),
						Link:        fmt.Sprintf("https://ci.chromium.org/ui/p/%s/builders/%s/%s", project, builder.Bucket, builder.Name),
						Description: healthStatus.Description,
					})
			} else {
				unhealthyBuilders = append(unhealthyBuilders,
					BuilderInfo{
						Name:        fmt.Sprintf("%s.%s:%s", project, builder.Bucket, builder.Name),
						Link:        fmt.Sprintf("https://ci.chromium.org/ui/p/%s/builders/%s/%s", project, builder.Bucket, builder.Name),
						Description: healthStatus.Description,
					})
			}
		}
		unhealthyBuilderCount := len(unhealthyBuilders)

		// Check if we need to send an email if all builders are healthy.
		if unhealthyBuilderCount == 0 && !builderHealthNotifier.NotifyAllHealthy {
			logging.Debugf(c, "Got 0 unhealthy builders and notify_all_healthy is set to %s", builderHealthNotifier.NotifyAllHealthy)
			continue
		}

		task := &internal.EmailTask{
			Recipients: []string{email},
			Subject:    fmt.Sprintf("Builder Health For %s - %d of %d Are in Bad Health", email, unhealthyBuilderCount, builderCount),
			BodyGzip:   generateEmail(c, email, unhealthyBuilderCount, builderCount, generateBuilderDescriptionHTML(unhealthyBuilders, healthyBuilders, unknownHealthBuilders), "https://chromium.googlesource.com/chromium/src/+/HEAD/docs/infra/builder_health_indicators.md"),
		}
		tasks[email] = task
	}
	return tasks, nil
}
