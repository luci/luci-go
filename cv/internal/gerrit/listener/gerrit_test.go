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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

type testScheduler struct {
	tasks []*changelist.UpdateCLTask
}

func (sch *testScheduler) Schedule(_ context.Context, tsk *changelist.UpdateCLTask) error {
	sch.tasks = append(sch.tasks, tsk)
	return nil
}

func TestGerrit(t *testing.T) {
	t.Parallel()

	ftt.Run("gerritSubscriber", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		client, closeFn := mockPubSub(t, ctx)
		defer closeFn()
		finder := &projectFinder{
			isListenerEnabled: func(string) bool { return true },
		}
		settings := &listenerpb.Settings_GerritSubscription{
			Host:          "example.org",
			MessageFormat: listenerpb.Settings_GerritSubscription_JSON,
		}

		t.Run("create a reference to subscription", func(t *ftt.Test) {
			sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)
			assert.Loosely(t, sber.sub.ID(), should.Equal("example.org"))
			settings.SubscriptionId = "my-sub"
			sber = newGerritSubscriber(client, &testScheduler{}, finder, settings)
			assert.Loosely(t, sber.sub.ID(), should.Equal("my-sub"))

			t.Run("with receive settings", func(t *ftt.Test) {
				settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines:          defaultNumGoroutines + 1,
					MaxOutstandingMessages: defaultMaxOutstandingMessages + 1,
				}
				sber = newGerritSubscriber(client, &testScheduler{}, finder, settings)
				assert.Loosely(t, sber.sameReceiveSettings(ctx, settings.ReceiveSettings), should.BeTrue)
			})
		})

		t.Run("sameGerritSubscriberSettings", func(t *ftt.Test) {
			settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
				NumGoroutines:          1,
				MaxOutstandingMessages: 2,
			}
			sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)
			check := func() bool {
				return sameGerritSubscriberSettings(ctx, sber, settings)
			}
			assert.Loosely(t, check(), should.BeTrue)

			t.Run("with different receiver settings", func(t *ftt.Test) {
				settings.ReceiveSettings.NumGoroutines++
				assert.Loosely(t, check(), should.BeFalse)
				settings.ReceiveSettings.NumGoroutines--
				settings.ReceiveSettings.MaxOutstandingMessages++
				assert.Loosely(t, check(), should.BeFalse)
				settings.ReceiveSettings.MaxOutstandingMessages--
				assert.Loosely(t, check(), should.BeTrue)
			})
			t.Run("with different subscription ID", func(t *ftt.Test) {
				settings.SubscriptionId = "new-sub"
				assert.Loosely(t, check(), should.BeFalse)
			})
			t.Run("with different host", func(t *ftt.Test) {
				settings.Host = "example.org"
				settings.SubscriptionId = "example-sub"
				sber := newGerritSubscriber(client, &testScheduler{}, finder, settings)

				// same subscription ID, but different host.
				settings.Host = "example-2.org"
				assert.Loosely(t, sameGerritSubscriberSettings(ctx, sber, settings), should.BeFalse)
			})
		})

		t.Run("processes", func(t *ftt.Test) {
			sch := &testScheduler{}
			msg := &pubsub.Message{}
			createTestLUCIProject(t, ctx, "chromium", "https://example.org/", "abc/foo")
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
				assert.Loosely(t, sber.proc.process(ctx, msg), should.BeNil)
				return sber
			}

			t.Run("empty", func(t *ftt.Test) {
				process()
				assert.Loosely(t, sch.tasks, should.HaveLength(0))
			})
			t.Run("message from an unwatched repo", func(t *ftt.Test) {
				cfg := &listenerpb.Settings{
					DisabledProjectRegexps: []string{"chromium"},
					GerritSubscriptions:    []*listenerpb.Settings_GerritSubscription{settings},
				}
				assert.Loosely(t, finder.reload(cfg), should.BeNil)
				msg.Data = payload
				process()
				assert.Loosely(t, sch.tasks, should.HaveLength(0))
			})
			t.Run("message from a watched repo", func(t *ftt.Test) {
				check := func(t testing.TB) {
					process()
					assert.Loosely(t, sch.tasks, should.Match([]*changelist.UpdateCLTask{
						{
							LuciProject: "chromium",
							ExternalId:  "gerrit/example.org/123",
							Requester:   changelist.UpdateCLTask_PUBSUB_POLL,
							Hint:        &changelist.UpdateCLTask_Hint{MetaRevId: "feas"},
						},
					}))
				}

				t.Run("in json", func(t *ftt.Test) {
					msg.Data = payload
					check(t)
				})
				t.Run("in binary", func(t *ftt.Test) {
					event := &gerritpb.SourceRepoEvent{}
					assert.Loosely(t, protojson.Unmarshal(payload, event), should.BeNil)
					bin, err := proto.Marshal(event)
					assert.NoErr(t, err)
					msg.Data = bin
					settings.MessageFormat = listenerpb.Settings_GerritSubscription_PROTO_BINARY
					check(t)
				})
			})

			t.Run("panic for an unknown enum", func(t *ftt.Test) {
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
