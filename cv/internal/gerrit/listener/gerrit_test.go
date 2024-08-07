// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testScheduler struct {
	tasks []*changelist.UpdateCLTask
}

func (sch *testScheduler) Schedule(_ context.Context, t *changelist.UpdateCLTask) error {
	sch.tasks = append(sch.tasks, t)
	return nil
}

func TestGerrit(t *testing.T) {
	t.Parallel()

	Convey("gerritSubscriber", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		finder := &projectFinder{
			isListenerEnabled: func(string) bool { return true },
		}
		settings := &listenerpb.Settings_GerritSubscription{
			Host:          "example.org",
			MessageFormat: listenerpb.Settings_GerritSubscription_JSON,
		}

		Convey("create a reference to subscription", func() {
			sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)
			So(sber.sub.ID(), ShouldEqual, "example.org")
			settings.SubscriptionId = "my-sub"
			sber = newGerritSubscriber(client, &testScheduler{}, finder, settings)
			So(sber.sub.ID(), ShouldEqual, "my-sub")

			Convey("with receive settings", func() {
				settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines:          defaultNumGoroutines + 1,
					MaxOutstandingMessages: defaultMaxOutstandingMessages + 1,
				}
				sber = newGerritSubscriber(client, &testScheduler{}, finder, settings)
				So(sber.sameReceiveSettings(ctx, settings.ReceiveSettings), ShouldBeTrue)
			})
		})

		Convey("sameGerritSubscriberSettings", func() {
			settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
				NumGoroutines:          1,
				MaxOutstandingMessages: 2,
			}
			sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)
			check := func() bool {
				return sameGerritSubscriberSettings(ctx, sber, settings)
			}
			So(check(), ShouldBeTrue)

			Convey("with different receiver settings", func() {
				settings.ReceiveSettings.NumGoroutines++
				So(check(), ShouldBeFalse)
				settings.ReceiveSettings.NumGoroutines--
				settings.ReceiveSettings.MaxOutstandingMessages++
				So(check(), ShouldBeFalse)
				settings.ReceiveSettings.MaxOutstandingMessages--
				So(check(), ShouldBeTrue)
			})
			Convey("with different subscription ID", func() {
				settings.SubscriptionId = "new-sub"
				So(check(), ShouldBeFalse)
			})
			Convey("with different host", func() {
				settings.Host = "example.org"
				settings.SubscriptionId = "example-sub"
				sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)

				// same subscription ID, but different host.
				settings.Host = "example-2.org"
				So(sameGerritSubscriberSettings(ctx, sber, settings), ShouldBeFalse)
			})
		})

		Convey("processes", func() {
			sch := &testScheduler{}
			msg := &pubsub.Message{}
			createTestLUCIProject(ctx, "chromium", "https://example.org/", "abc/foo")
			payload := []byte(`{
				"name": "projects/project/repos/abc/foo",
				"url": "https://example.org/p/project/r/abc/foo",
				"eventTime": "2022-10-03T16:47:53.728031Z",
				"refUpdateEvent": {
					"email": "someone@example.org",
					"refUpdates": {
						"refs/changes/1/123/meta": {
							"refName": "refs/changes/1/123/meta",
							"updateType": "UPDATE_FAST_FORWARD",
							"oldId": "deaf",
							"newId": "feas"
						}
					}
				}
			}`)
			process := func() *subscriber {
				sber := newGerritSubscriber(client, sch, finder, settings)
				So(sber.proc.process(ctx, msg), ShouldBeNil)
				return sber
			}

			Convey("empty", func() {
				process()
				So(sch.tasks, ShouldHaveLength, 0)
			})
			Convey("message from an unwatched repo", func() {
				cfg := &listenerpb.Settings{
					DisabledProjectRegexps: []string{"chromium"},
					GerritSubscriptions:    []*listenerpb.Settings_GerritSubscription{settings},
				}
				So(finder.reload(cfg), ShouldBeNil)
				msg.Data = payload
				process()
				So(sch.tasks, ShouldHaveLength, 0)
			})
			Convey("message from a watched repo", func() {
				Convey("in json", func() {
					msg.Data = payload
				})
				Convey("in binary", func() {
					event := &gerritpb.SourceRepoEvent{}
					So(protojson.Unmarshal(payload, event), ShouldBeNil)
					bin, err := proto.Marshal(event)
					So(err, ShouldBeNil)
					msg.Data = bin
					settings.MessageFormat = listenerpb.Settings_GerritSubscription_PROTO_BINARY
				})
				process()
				So(sch.tasks, ShouldResembleProto, []*changelist.UpdateCLTask{
					{
						LuciProject: "chromium",
						ExternalId:  "gerrit/example.org/123",
						Requester:   changelist.UpdateCLTask_PUBSUB_POLL,
						Hint:        &changelist.UpdateCLTask_Hint{MetaRevId: "feas"},
					},
				})
			})

			Convey("panic for an unknown enum", func() {
				// This test is to ensure that gerritProcessor.process() handles
				// all the enums defined for gerrit_subscription.message_format.
				//
				// If this test ever panics, it means that a new enum was added
				// but gerritProcessor.process() was not updated to handle
				// the new format. Please fix.
				msg.Data = payload
				for name, val := range listenerpb.Settings_GerritSubscription_MessageFormat_value {
					switch name {
					case listenerpb.Settings_GerritSubscription_MESSAGE_FORMAT_UNSPECIFIED.String():
					case listenerpb.Settings_GerritSubscription_JSON.String():
					case listenerpb.Settings_GerritSubscription_PROTO_BINARY.String():
					default:
						// this must cause a panic.
						settings.MessageFormat = listenerpb.Settings_GerritSubscription_MessageFormat(val)
						process()
						panic(fmt.Errorf(
							"gerritProcessor.process() didn't panic for an unknown enum %s",
							name,
						))
					}
				}
			})
		})
	})
}
