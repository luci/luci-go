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
	"compress/gzip"
	"context"
	"io"
	"sort"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"
)

func TestNotify(t *testing.T) {
	t.Parallel()

	ftt.Run("ShouldNotify", t, func(t *ftt.Test) {
		n := &notifypb.Notification{}
		n.OnOccurrence = []buildbucketpb.Status{}
		n.OnNewStatus = []buildbucketpb.Status{}

		const (
			unspecified  = buildbucketpb.Status_STATUS_UNSPECIFIED
			success      = buildbucketpb.Status_SUCCESS
			failure      = buildbucketpb.Status_FAILURE
			infraFailure = buildbucketpb.Status_INFRA_FAILURE
		)

		successfulBuild := &buildbucketpb.Build{Status: success}
		failedBuild := &buildbucketpb.Build{Status: failure}
		infraFailedBuild := &buildbucketpb.Build{Status: infraFailure}

		// Helper wrapper which discards the steps and just returns the bool.
		s := func(oldStatus buildbucketpb.Status, newBuild *buildbucketpb.Build) bool {
			should, _ := ShouldNotify(context.Background(), n, oldStatus, newBuild)
			return should
		}

		t.Run("Success", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, success)

			assert.Loosely(t, s(unspecified, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(success, successfulBuild), should.BeTrue)
		})

		t.Run("Failure", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, failure)

			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
		})

		t.Run("InfraFailure", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)

			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
		})

		t.Run("Failure and InfraFailure", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, failure, infraFailure)

			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
		})

		t.Run("New Failure", func(t *ftt.Test) {
			n.OnNewStatus = append(n.OnNewStatus, failure)

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("New InfraFailure", func(t *ftt.Test) {
			n.OnNewStatus = append(n.OnNewStatus, infraFailure)

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeFalse)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("New Failure and new InfraFailure", func(t *ftt.Test) {
			n.OnNewStatus = append(n.OnNewStatus, failure, infraFailure)

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("InfraFailure and new Failure and new Success", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)
			n.OnNewStatus = append(n.OnNewStatus, failure, success)

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeTrue)
		})

		t.Run("Failure with step regex", func(t *ftt.Test) {
			n.OnOccurrence = append(n.OnOccurrence, failure)
			n.FailedStepRegexp = "yes"
			n.FailedStepRegexpExclude = "no"

			shouldHaveStep := func(t testing.TB, oldStatus buildbucketpb.Status, newBuild *buildbucketpb.Build, stepName string) {
				t.Helper()
				matched, steps := ShouldNotify(context.Background(), n, oldStatus, newBuild)

				assert.Loosely(t, matched, should.BeTrue, truth.LineContext())
				assert.Loosely(t, steps, should.HaveLength(1), truth.LineContext())
				assert.Loosely(t, steps[0].Name, should.Equal(stepName), truth.LineContext())
			}

			assert.Loosely(t, s(success, failedBuild), should.BeFalse)
			shouldHaveStep(t, success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: failure,
					},
				},
			}, "yes")
			assert.Loosely(t, s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: success,
					},
				},
			}), should.BeFalse)

			assert.Loosely(t, s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "no",
						Status: failure,
					},
				},
			}), should.BeFalse)
			assert.Loosely(t, s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: success,
					},
					{
						Name:   "no",
						Status: failure,
					},
				},
			}), should.BeFalse)
			shouldHaveStep(t, success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: failure,
					},
					{
						Name:   "no",
						Status: failure,
					},
				},
			}, "yes")
			assert.Loosely(t, s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yesno",
						Status: failure,
					},
				},
			}), should.BeFalse)
			shouldHaveStep(t, success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yesno",
						Status: failure,
					},
					{
						Name:   "yes",
						Status: failure,
					},
				},
			}, "yes")
		})

		t.Run("OnSuccess deprecated", func(t *ftt.Test) {
			n.OnSuccess = true

			assert.Loosely(t, s(success, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(success, failedBuild), should.BeFalse)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("OnFailure deprecated", func(t *ftt.Test) {
			n.OnFailure = true

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("OnChange deprecated", func(t *ftt.Test) {
			n.OnChange = true

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(failure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})

		t.Run("OnNewFailure deprecated", func(t *ftt.Test) {
			n.OnNewFailure = true

			assert.Loosely(t, s(success, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(success, failedBuild), should.BeTrue)
			assert.Loosely(t, s(success, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(failure, failedBuild), should.BeFalse)
			assert.Loosely(t, s(failure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(infraFailure, failedBuild), should.BeTrue)
			assert.Loosely(t, s(infraFailure, infraFailedBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, successfulBuild), should.BeFalse)
			assert.Loosely(t, s(unspecified, failedBuild), should.BeTrue)
			assert.Loosely(t, s(unspecified, infraFailedBuild), should.BeFalse)
		})
	})

	ftt.Run("Notify", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify")
		c = caching.WithEmptyProcessCache(c)
		c = clock.Set(c, testclock.New(testclock.TestRecentTimeUTC))
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)

		build := &Build{
			Build: buildbucketpb.Build{
				Id: 54,
				Builder: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				Status: buildbucketpb.Status_SUCCESS,
			},
		}

		// Put Project and EmailTemplate entities.
		project := &config.Project{Name: "chromium", Revision: "deadbeef"}
		templates := []*config.EmailTemplate{
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}}",
			},
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "non-default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed from non-default template",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}} from non-default template",
			},
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "with-steps",
				SubjectTextTemplate: "Subject {{ stepNames .MatchingFailedSteps }}",
				BodyHTMLTemplate:    "Body {{ stepNames .MatchingFailedSteps }}",
			},
		}
		assert.Loosely(t, datastore.Put(c, project, templates), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		t.Run("createEmailTasks", func(t *ftt.Test) {
			emailNotify := []EmailNotify{
				{
					Email: "jane@example.com",
				},
				{
					Email: "john@example.com",
				},
				{
					Email:    "don@example.com",
					Template: "non-default",
				},
				{
					Email:    "juan@example.com",
					Template: "with-steps",
					MatchingSteps: []*buildbucketpb.Step{
						{
							Name: "step name",
						},
					},
				},
			}

			tasks, err := createEmailTasks(c, emailNotify, &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build:               &build.Build,
				OldStatus:           buildbucketpb.Status_SUCCESS,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks, should.HaveLength(4))

			task := tasks["54-default-jane@example.com"]
			assert.Loosely(t, task.Recipients, should.Resemble([]string{"jane@example.com"}))
			assert.Loosely(t, task.Subject, should.Equal("Build 54 completed"))
			assert.Loosely(t, decompress(t, task.BodyGzip), should.Equal("Build 54 completed with status SUCCESS"))

			task = tasks["54-default-john@example.com"]
			assert.Loosely(t, task.Recipients, should.Resemble([]string{"john@example.com"}))
			assert.Loosely(t, task.Subject, should.Equal("Build 54 completed"))
			assert.Loosely(t, decompress(t, task.BodyGzip), should.Equal("Build 54 completed with status SUCCESS"))

			task = tasks["54-non-default-don@example.com"]
			assert.Loosely(t, task.Recipients, should.Resemble([]string{"don@example.com"}))
			assert.Loosely(t, task.Subject, should.Equal("Build 54 completed from non-default template"))
			assert.Loosely(t, decompress(t, task.BodyGzip), should.Equal("Build 54 completed with status SUCCESS from non-default template"))

			task = tasks["54-with-steps-juan@example.com"]
			assert.Loosely(t, task.Recipients, should.Resemble([]string{"juan@example.com"}))
			assert.Loosely(t, task.Subject, should.Equal(`Subject "step name"`))
			assert.Loosely(t, decompress(t, task.BodyGzip), should.Equal("Body &#34;step name&#34;"))
		})

		t.Run("createEmailTasks with dup notifies", func(t *ftt.Test) {
			emailNotify := []EmailNotify{
				{
					Email: "jane@example.com",
				},
				{
					Email: "jane@example.com",
				},
			}
			tasks, err := createEmailTasks(c, emailNotify, &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build:               &build.Build,
				OldStatus:           buildbucketpb.Status_SUCCESS,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks, should.HaveLength(1))
		})
	})
}

func TestComputeRecipients(t *testing.T) {
	ftt.Run("ComputeRecipients", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify")
		c = caching.WithEmptyProcessCache(c)
		c = clock.Set(c, testclock.New(testclock.TestRecentTimeUTC))
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)

		oncallers := map[string]string{
			"https://rota-ng.appspot.com/legacy/sheriff.json": `{
				"updated_unix_timestamp": 1582692124,
				"emails": [
					"sheriff1@google.com",
					"sheriff2@google.com",
					"sheriff3@google.com",
					"sheriff4@google.com"
				]
			}`,
			"https://rota-ng.appspot.com/legacy/sheriff_ios.json": `{
				"updated_unix_timestamp": 1582692124,
				"emails": [
					"sheriff5@google.com",
					"sheriff6@google.com"
				]
			}`,
			"https://rotations.site/bad.json": "@!(*",
		}
		fetch := func(_ context.Context, url string) ([]byte, error) {
			if s, e := oncallers[url]; e {
				return []byte(s), nil
			} else {
				return []byte(""), errors.New("Key not present")
			}
		}

		t.Run("ComputeRecipients fetches all sheriffs", func(t *ftt.Test) {
			n := []ToNotify{
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff.json"},
						},
					},
				},
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_ios_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff_ios.json"},
						},
					},
				},
			}
			emails := computeRecipientsInternal(c, n, nil, nil, fetch)

			// ComputeRecipients is concurrent, hence we have no guarantees as to the order.
			// So we sort here to ensure a consistent ordering.
			sort.Slice(emails, func(i, j int) bool {
				return emails[i].Email < emails[j].Email
			})

			assert.Loosely(t, emails, should.Resemble([]EmailNotify{
				{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff5@google.com",
					Template: "sheriff_ios_template",
				},
				{
					Email:    "sheriff6@google.com",
					Template: "sheriff_ios_template",
				},
			}))
		})

		t.Run("ComputeRecipients drops missing", func(t *ftt.Test) {
			n := []ToNotify{
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff.json"},
						},
					},
				},
				{
					Notification: &notifypb.Notification{
						Template: "what",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://somerandom.url/huh.json"},
						},
					},
				},
			}
			emails := computeRecipientsInternal(c, n, nil, nil, fetch)

			// ComputeRecipients is concurrent, hence we have no guarantees as to the order.
			// So we sort here to ensure a consistent ordering.
			sort.Slice(emails, func(i, j int) bool {
				return emails[i].Email < emails[j].Email
			})

			assert.Loosely(t, emails, should.Resemble([]EmailNotify{
				{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
			}))
		})

		t.Run("ComputeRecipients includes static emails", func(t *ftt.Test) {
			n := []ToNotify{
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff.json"},
						},
					},
				},
				{
					Notification: &notifypb.Notification{
						Template: "other_template",
						Email: &notifypb.Notification_Email{
							Recipients: []string{"someone@google.com"},
						},
					},
				},
			}
			emails := computeRecipientsInternal(c, n, nil, nil, fetch)

			// ComputeRecipients is concurrent, hence we have no guarantees as to the order.
			// So we sort here to ensure a consistent ordering.
			sort.Slice(emails, func(i, j int) bool {
				return emails[i].Email < emails[j].Email
			})

			assert.Loosely(t, emails, should.Resemble([]EmailNotify{
				{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "someone@google.com",
					Template: "other_template",
				},
			}))
		})

		t.Run("ComputeRecipients drops bad JSON", func(t *ftt.Test) {
			n := []ToNotify{
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff.json"},
						},
					},
				},
				{
					Notification: &notifypb.Notification{
						Template: "bad JSON",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rotations.site/bad.json"},
						},
					},
				},
			}
			emails := computeRecipientsInternal(c, n, nil, nil, fetch)

			// ComputeRecipients is concurrent, hence we have no guarantees as to the order.
			// So we sort here to ensure a consistent ordering.
			sort.Slice(emails, func(i, j int) bool {
				return emails[i].Email < emails[j].Email
			})

			assert.Loosely(t, emails, should.Resemble([]EmailNotify{
				{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
			}))
		})

		t.Run("ComputeRecipients propagates MatchingSteps", func(t *ftt.Test) {
			sheriffSteps := []*buildbucketpb.Step{
				{
					Name: "sheriff step",
				},
			}
			otherSteps := []*buildbucketpb.Step{
				{
					Name: "other step",
				},
			}
			n := []ToNotify{
				{
					Notification: &notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotationUrls: []string{"https://rota-ng.appspot.com/legacy/sheriff.json"},
						},
					},
					MatchingSteps: sheriffSteps,
				},
				{
					Notification: &notifypb.Notification{
						Template: "other_template",
						Email: &notifypb.Notification_Email{
							Recipients: []string{"someone@google.com"},
						},
					},
					MatchingSteps: otherSteps,
				},
			}
			emails := computeRecipientsInternal(c, n, nil, nil, fetch)

			// ComputeRecipients is concurrent, hence we have no guarantees as to the order.
			// So we sort here to ensure a consistent ordering.
			sort.Slice(emails, func(i, j int) bool {
				return emails[i].Email < emails[j].Email
			})

			assert.Loosely(t, emails, should.Resemble([]EmailNotify{
				{
					Email:         "sheriff1@google.com",
					Template:      "sheriff_template",
					MatchingSteps: sheriffSteps,
				},
				{
					Email:         "sheriff2@google.com",
					Template:      "sheriff_template",
					MatchingSteps: sheriffSteps,
				},
				{
					Email:         "sheriff3@google.com",
					Template:      "sheriff_template",
					MatchingSteps: sheriffSteps,
				},
				{
					Email:         "sheriff4@google.com",
					Template:      "sheriff_template",
					MatchingSteps: sheriffSteps,
				},
				{
					Email:         "someone@google.com",
					Template:      "other_template",
					MatchingSteps: otherSteps,
				},
			}))
		})
	})
}

func decompress(t testing.TB, gzipped []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(gzipped))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	buf, err := io.ReadAll(r)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return string(buf)
}
