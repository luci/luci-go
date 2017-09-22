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
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/gae/service/user"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/stringset"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/buildbucket"
	"go.chromium.org/luci/luci_notify/config"
)

func loadConfig(filename string) (*notifyConfig.ProjectConfig, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fullname := fmt.Sprintf("%s/../api/config/testdata/%s.cfg", dir, filename)
	buf, err := ioutil.ReadFile(fullname)
	if err != nil {
		return nil, err
	}
	project := notifyConfig.ProjectConfig{}
	if err := proto.UnmarshalText(string(buf[:]), &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func extractNotifiers(c context.Context, cfgName string, cfg *notifyConfig.ProjectConfig) []*config.Notifier {
	var notifiers []*config.Notifier
	parentKey := datastore.MakeKey(c, "Project", cfgName)
	for _, n := range cfg.Notifiers {
		notifiers = append(notifiers, config.NewNotifier(parentKey, n))
	}
	return notifiers
}

// mockTimestamp returns a new UTC-based microsecond Unix timestamp for
// the given date and time.
func mockTimestamp(yyyy, mm, dd, hour, min, sec int) int64 {
	l, err := time.LoadLocation("")
	if err != nil {
		log.Fatalf("failed to get timezone")
	}
	return time.Date(yyyy, time.Month(mm), dd, hour, min, sec, 0, l).UnixNano() / 1000
}

func createBuildInfo(time int64, result string) *buildbucket.BuildInfo {
	return &buildbucket.BuildInfo{
		Build: bbapi.ApiCommonBuildMessage{
			CreatedTs: time,
			Result:    result,
		},
		Hostname: "bb.com",
		Parameters: buildbucket.BuildParameters{
			BuilderName: "test-builder",
		},
	}
}

func TestNotification(t *testing.T) {
	Convey(`Test Environment for Notification`, t, func() {
		cfgName := "basic"
		cfg, err := loadConfig(cfgName)
		So(err, ShouldBeNil)
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		notifiers := extractNotifiers(c, cfgName, cfg)
		goodBuild := createBuildInfo(mockTimestamp(2015, 2, 3, 12, 55, 2), "SUCCESS")
		badBuild := createBuildInfo(mockTimestamp(2015, 2, 3, 12, 56, 2), "FAILURE")
		goodBuilder := &Builder{
			LastBuildTime:   mockTimestamp(2015, 2, 3, 12, 54, 3),
			LastBuildResult: "SUCCESS",
		}
		badBuilder := &Builder{
			LastBuildTime:   mockTimestamp(2015, 2, 3, 12, 54, 3),
			LastBuildResult: "FAILURE",
		}

		test := func(build *buildbucket.BuildInfo, builder *Builder, emailExpect ...string) {
			// Test creating the notification.
			n := CreateNotification(notifiers, build, builder)
			So(n, ShouldNotBeNil)
			actualRecipients := stringset.NewFromSlice(n.EmailRecipients...)
			expectRecipients := stringset.NewFromSlice(emailExpect...)
			So(actualRecipients, ShouldResemble, expectRecipients)
			So(n.Build, ShouldEqual, build)
			So(n.Builder, ShouldEqual, builder)

			// Login and send email.
			user.GetTestable(c).Login("noreply@luci-notify-dev.appspotmail.com", "", false)
			So(n.Dispatch(c), ShouldBeNil)

			// Make sure an email was sent, but don't test its exact contents.
			So(len(mail.GetTestable(c).SentMessages()), ShouldEqual, 1)
		}

		Convey(`empty`, func() {
			creationNoneTest := func(build *buildbucket.BuildInfo, builder *Builder) {
				n := CreateNotification([]*config.Notifier{}, build, builder)
				So(n, ShouldBeNil)
			}
			creationNoneTest(goodBuild, goodBuilder)
			creationNoneTest(goodBuild, badBuilder)
			creationNoneTest(badBuild, goodBuilder)
			creationNoneTest(badBuild, badBuilder)
		})

		Convey(`on success`, func() {
			test(
				goodBuild,
				goodBuilder,
				"test-success@example.com",
			)
		})

		Convey(`on failure`, func() {
			test(
				badBuild,
				badBuilder,
				"test-failure@example.com",
			)
		})

		Convey(`on change to failure`, func() {
			test(
				badBuild,
				goodBuilder,
				"test-failure@example.com",
				"test-change@example.com",
			)
		})

		Convey(`on change to success`, func() {
			test(
				goodBuild,
				badBuilder,
				"test-success@example.com",
				"test-change@example.com",
			)
		})
	})
}
