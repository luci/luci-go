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
	"io/ioutil"
	"sort"
	"testing"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotify(t *testing.T) {
	Convey("Notify", t, func() {
		c := gaetesting.TestingContextWithAppID("luci-notify")
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
			}
			tasks, err := createEmailTasks(c, emailNotify, &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build:               &build.Build,
				OldStatus:           buildbucketpb.Status_SUCCESS,
			})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 3)

			t := tasks[0].Payload.(*internal.EmailTask)
			So(tasks[0].DeduplicationKey, ShouldEqual, "54-default-jane@example.com")
			So(t.Recipients, ShouldResemble, []string{"jane@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks[1].Payload.(*internal.EmailTask)
			So(tasks[1].DeduplicationKey, ShouldEqual, "54-default-john@example.com")
			So(t.Recipients, ShouldResemble, []string{"john@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks[2].Payload.(*internal.EmailTask)
			So(tasks[2].DeduplicationKey, ShouldEqual, "54-non-default-don@example.com")
			So(t.Recipients, ShouldResemble, []string{"don@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed from non-default template")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS from non-default template")
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
		c := gaetesting.TestingContextWithAppID("luci-notify")
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
			"https://rota-ng.appspot.com/legacy/bad.json": "@!(*",
		}
		fetch := func(_ context.Context, url string) ([]byte, error) {
			if s, e := oncallers[url]; e {
				return []byte(s), nil
			} else {
				return []byte(""), errors.New("Key not present")
			}
		}

		Convey("ComputeRecipients fetches all sheriffs", func() {
			n := notifypb.Notifications{
				Notifications: []*notifypb.Notification{
					&notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"sheriff"},
						},
					},
					&notifypb.Notification{
						Template: "sheriff_ios_template",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"sheriff_ios"},
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
				EmailNotify{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff5@google.com",
					Template: "sheriff_ios_template",
				},
				EmailNotify{
					Email:    "sheriff6@google.com",
					Template: "sheriff_ios_template",
				},
			})
		})

		Convey("ComputeRecipients drops missing", func() {
			n := notifypb.Notifications{
				Notifications: []*notifypb.Notification{
					&notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"sheriff"},
						},
					},
					&notifypb.Notification{
						Template: "what",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"huh"},
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
				EmailNotify{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
			})
		})

		Convey("ComputeRecipients includes static emails", func() {
			n := notifypb.Notifications{
				Notifications: []*notifypb.Notification{
					&notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"sheriff"},
						},
					},
					&notifypb.Notification{
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
				EmailNotify{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "someone@google.com",
					Template: "other_template",
				},
			})
		})

		Convey("ComputeRecipients drops bad JSON", func() {
			n := notifypb.Notifications{
				Notifications: []*notifypb.Notification{
					&notifypb.Notification{
						Template: "sheriff_template",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"sheriff"},
						},
					},
					&notifypb.Notification{
						Template: "bad JSON",
						Email: &notifypb.Notification_Email{
							RotaNgRotations: []string{"bad"},
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
				EmailNotify{
					Email:    "sheriff1@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff2@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff3@google.com",
					Template: "sheriff_template",
				},
				EmailNotify{
					Email:    "sheriff4@google.com",
					Template: "sheriff_template",
				},
			})
		})
	})
}

func decompress(gzipped []byte) string {
	r, err := gzip.NewReader(bytes.NewReader(gzipped))
	So(err, ShouldBeNil)
	buf, err := ioutil.ReadAll(r)
	So(err, ShouldBeNil)
	return string(buf)
}
