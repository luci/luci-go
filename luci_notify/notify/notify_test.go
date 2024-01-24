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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotify(t *testing.T) {
	t.Parallel()

	Convey("ShouldNotify", t, func() {
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

		Convey("Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, success)

			So(s(unspecified, successfulBuild), ShouldBeTrue)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(success, successfulBuild), ShouldBeTrue)
		})

		Convey("Failure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure)

			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeTrue)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(success, successfulBuild), ShouldBeFalse)
		})

		Convey("InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)

			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeTrue)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(s(success, successfulBuild), ShouldBeFalse)
		})

		Convey("Failure and InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure, infraFailure)

			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeTrue)
			So(s(unspecified, infraFailedBuild), ShouldBeTrue)
			So(s(failure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(s(success, successfulBuild), ShouldBeFalse)
		})

		Convey("New Failure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure)

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeFalse)
			So(s(failure, successfulBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeFalse)
			So(s(infraFailure, successfulBuild), ShouldBeFalse)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("New InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, infraFailure)

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeFalse)
			So(s(success, infraFailedBuild), ShouldBeTrue)
			So(s(failure, successfulBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeTrue)
			So(s(infraFailure, successfulBuild), ShouldBeFalse)
			So(s(infraFailure, failedBuild), ShouldBeFalse)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("New Failure and new InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure, infraFailure)

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeTrue)
			So(s(failure, successfulBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeTrue)
			So(s(infraFailure, successfulBuild), ShouldBeFalse)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("InfraFailure and new Failure and new Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)
			n.OnNewStatus = append(n.OnNewStatus, failure, success)

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeTrue)
			So(s(failure, successfulBuild), ShouldBeTrue)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeTrue)
			So(s(infraFailure, successfulBuild), ShouldBeTrue)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeTrue)
		})

		Convey("Failure with step regex", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure)
			n.FailedStepRegexp = "yes"
			n.FailedStepRegexpExclude = "no"

			shouldHaveStep := func(oldStatus buildbucketpb.Status, newBuild *buildbucketpb.Build, stepName string) {
				should, steps := ShouldNotify(context.Background(), n, oldStatus, newBuild)

				So(should, ShouldBeTrue)
				So(steps, ShouldHaveLength, 1)
				So(steps[0].Name, ShouldEqual, stepName)
			}

			So(s(success, failedBuild), ShouldBeFalse)
			shouldHaveStep(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: failure,
					},
				},
			}, "yes")
			So(s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yes",
						Status: success,
					},
				},
			}), ShouldBeFalse)

			So(s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "no",
						Status: failure,
					},
				},
			}), ShouldBeFalse)
			So(s(success, &buildbucketpb.Build{
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
			}), ShouldBeFalse)
			shouldHaveStep(success, &buildbucketpb.Build{
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
			So(s(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					{
						Name:   "yesno",
						Status: failure,
					},
				},
			}), ShouldBeFalse)
			shouldHaveStep(success, &buildbucketpb.Build{
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

		Convey("OnSuccess deprecated", func() {
			n.OnSuccess = true

			So(s(success, successfulBuild), ShouldBeTrue)
			So(s(success, failedBuild), ShouldBeFalse)
			So(s(success, infraFailedBuild), ShouldBeFalse)
			So(s(failure, successfulBuild), ShouldBeTrue)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeFalse)
			So(s(infraFailure, successfulBuild), ShouldBeTrue)
			So(s(infraFailure, failedBuild), ShouldBeFalse)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeTrue)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnFailure deprecated", func() {
			n.OnFailure = true

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeFalse)
			So(s(failure, successfulBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeTrue)
			So(s(failure, infraFailedBuild), ShouldBeFalse)
			So(s(infraFailure, successfulBuild), ShouldBeFalse)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeTrue)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnChange deprecated", func() {
			n.OnChange = true

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeTrue)
			So(s(failure, successfulBuild), ShouldBeTrue)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeTrue)
			So(s(infraFailure, successfulBuild), ShouldBeTrue)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeFalse)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnNewFailure deprecated", func() {
			n.OnNewFailure = true

			So(s(success, successfulBuild), ShouldBeFalse)
			So(s(success, failedBuild), ShouldBeTrue)
			So(s(success, infraFailedBuild), ShouldBeFalse)
			So(s(failure, successfulBuild), ShouldBeFalse)
			So(s(failure, failedBuild), ShouldBeFalse)
			So(s(failure, infraFailedBuild), ShouldBeFalse)
			So(s(infraFailure, successfulBuild), ShouldBeFalse)
			So(s(infraFailure, failedBuild), ShouldBeTrue)
			So(s(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(s(unspecified, successfulBuild), ShouldBeFalse)
			So(s(unspecified, failedBuild), ShouldBeTrue)
			So(s(unspecified, infraFailedBuild), ShouldBeFalse)
		})
	})

	Convey("Notify", t, func() {
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
		So(datastore.Put(c, project, templates), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("createEmailTasks", func() {
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
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 4)

			t := tasks["54-default-jane@example.com"]
			So(t.Recipients, ShouldResemble, []string{"jane@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks["54-default-john@example.com"]
			So(t.Recipients, ShouldResemble, []string{"john@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks["54-non-default-don@example.com"]
			So(t.Recipients, ShouldResemble, []string{"don@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed from non-default template")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS from non-default template")

			t = tasks["54-with-steps-juan@example.com"]
			So(t.Recipients, ShouldResemble, []string{"juan@example.com"})
			So(t.Subject, ShouldEqual, `Subject "step name"`)
			So(decompress(t.BodyGzip), ShouldEqual, "Body &#34;step name&#34;")
		})

		Convey("createEmailTasks with dup notifies", func() {
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
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 1)
		})
	})
}

func TestComputeRecipients(t *testing.T) {
	Convey("ComputeRecipients", t, func() {
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

		Convey("ComputeRecipients fetches all sheriffs", func() {
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

			So(emails, ShouldResemble, []EmailNotify{
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
			})
		})

		Convey("ComputeRecipients drops missing", func() {
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

			So(emails, ShouldResemble, []EmailNotify{
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
			})
		})

		Convey("ComputeRecipients includes static emails", func() {
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

			So(emails, ShouldResemble, []EmailNotify{
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
			})
		})

		Convey("ComputeRecipients drops bad JSON", func() {
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

			So(emails, ShouldResemble, []EmailNotify{
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
			})
		})

		Convey("ComputeRecipients propagates MatchingSteps", func() {
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

			So(emails, ShouldResemble, []EmailNotify{
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
			})
		})
	})
}

func decompress(gzipped []byte) string {
	r, err := gzip.NewReader(bytes.NewReader(gzipped))
	So(err, ShouldBeNil)
	buf, err := io.ReadAll(r)
	So(err, ShouldBeNil)
	return string(buf)
}
