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

package subscriber

import (
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/caching"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
)

type fakeCLUpdater struct {
	tasks []*changelist.UpdateCLTask
}

func (updater *fakeCLUpdater) Schedule(_ context.Context, task *changelist.UpdateCLTask) error {
	updater.tasks = append(updater.tasks, task)
	return nil
}

func TestProcessPubSubMessage(t *testing.T) {
	t.Parallel()
	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	const luciProj = "test-luci-project"
	const gerritHost = "chromium-review.googlesource.com"
	prjcfgtest.Create(ctx, luciProj, &cfgpb.Config{})

	testcases := []struct {
		name          string
		event         *gerritpb.SourceRepoEvent
		format        string
		projectConfig *cfgpb.Config
		wantTasks     []*changelist.UpdateCLTask
	}{
		{
			name: "success with json",
			event: &gerritpb.SourceRepoEvent{
				Name: "projects/whatever/repos/test/repo",
				Event: &gerritpb.SourceRepoEvent_RefUpdateEvent_{
					RefUpdateEvent: &gerritpb.SourceRepoEvent_RefUpdateEvent{
						RefUpdates: map[string]*gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate{
							"refs/changes/12/123456/meta": {
								NewId: "new-id",
								OldId: "old-id",
							},
						},
					},
				},
			},
			format: jsonFormat,
			projectConfig: &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "main",
						Gerrit: []*cfgpb.ConfigGroup_Gerrit{
							{
								Url: "https://" + gerritHost,
								Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
									{
										Name:      "test/repo",
										RefRegexp: []string{"refs/heads/main"},
									},
								},
							},
						},
					},
				},
				GerritListenerType: cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB,
			},
			wantTasks: []*changelist.UpdateCLTask{
				{
					LuciProject: luciProj,
					ExternalId:  "gerrit/chromium-review.googlesource.com/123456",
					Requester:   changelist.UpdateCLTask_PUBSUB_PUSH,
					Hint:        &changelist.UpdateCLTask_Hint{MetaRevId: "new-id"},
				},
			},
		},
		{
			name: "success with binary",
			event: &gerritpb.SourceRepoEvent{
				Name: "projects/whatever/repos/test/repo",
				Event: &gerritpb.SourceRepoEvent_RefUpdateEvent_{
					RefUpdateEvent: &gerritpb.SourceRepoEvent_RefUpdateEvent{
						RefUpdates: map[string]*gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate{
							"refs/changes/12/123456/meta": {
								NewId: "new-id",
								OldId: "old-id",
							},
						},
					},
				},
			},
			format: protoBinaryFormat,
			projectConfig: &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "main",
						Gerrit: []*cfgpb.ConfigGroup_Gerrit{
							{
								Url: "https://" + gerritHost,
								Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
									{
										Name:      "test/repo",
										RefRegexp: []string{"refs/heads/main"},
									},
								},
							},
						},
					},
				},
				GerritListenerType: cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB,
			},
			wantTasks: []*changelist.UpdateCLTask{
				{
					LuciProject: luciProj,
					ExternalId:  "gerrit/chromium-review.googlesource.com/123456",
					Requester:   changelist.UpdateCLTask_PUBSUB_PUSH,
					Hint:        &changelist.UpdateCLTask_Hint{MetaRevId: "new-id"},
				},
			},
		},
		{
			name: "not watched by luci project",
			event: &gerritpb.SourceRepoEvent{
				Name: "projects/whatever/repos/another/repo",
				Event: &gerritpb.SourceRepoEvent_RefUpdateEvent_{
					RefUpdateEvent: &gerritpb.SourceRepoEvent_RefUpdateEvent{
						RefUpdates: map[string]*gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate{
							"refs/changes/12/123456/meta": {
								NewId: "new-id",
								OldId: "old-id",
							},
						},
					},
				},
			},
			format: protoBinaryFormat,
			projectConfig: &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "main",
						Gerrit: []*cfgpb.ConfigGroup_Gerrit{
							{
								Url: "https://" + gerritHost,
								Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
									{
										Name:      "test/repo",
										RefRegexp: []string{"refs/heads/main"},
									},
								},
							},
						},
					},
				},
				GerritListenerType: cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB,
			},
			wantTasks: nil,
		},
		{
			name: "luci project not using Pubsub",
			event: &gerritpb.SourceRepoEvent{
				Name: "projects/whatever/repos/test/repo",
				Event: &gerritpb.SourceRepoEvent_RefUpdateEvent_{
					RefUpdateEvent: &gerritpb.SourceRepoEvent_RefUpdateEvent{
						RefUpdates: map[string]*gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate{
							"refs/changes/12/123456/meta": {
								NewId: "new-id",
								OldId: "old-id",
							},
						},
					},
				},
			},
			format: protoBinaryFormat,
			projectConfig: &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "main",
						Gerrit: []*cfgpb.ConfigGroup_Gerrit{
							{
								Url: "https://" + gerritHost,
								Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
									{
										Name:      "test/repo",
										RefRegexp: []string{"refs/heads/main"},
									},
								},
							},
						},
					},
				},
				GerritListenerType: cfgpb.Config_GERRIT_LISTENER_TYPE_LEGACY_POLLER,
			},
			wantTasks: nil,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Name()
			ctx = caching.WithEmptyProcessCache(ctx)
			prjcfgtest.Update(ctx, luciProj, testcase.projectConfig)
			gobmaptest.Update(ctx, luciProj)
			updater := &fakeCLUpdater{}
			gs := &GerritSubscriber{CLUpdater: updater}
			assert.NoErr(t, gs.ProcessPubSubMessage(ctx, convertEventToPubSubPayload(t, testcase.event, testcase.format), gerritHost, testcase.format))
			assert.That(t, updater.tasks, should.Match(testcase.wantTasks))
		})
	}
}

func TestProcessEmptyPubSubMessage(t *testing.T) {
	t.Parallel()
	gs := &GerritSubscriber{}
	assert.NoErr(t, gs.ProcessPubSubMessage(context.Background(), common.PubSubMessagePayload{}, "chromium", jsonFormat))
}

func TestProcessPubSubMessageRequireHostName(t *testing.T) {
	t.Parallel()
	gs := &GerritSubscriber{}
	assert.ErrIsLike(t, gs.ProcessPubSubMessage(context.Background(), common.PubSubMessagePayload{}, "", jsonFormat), "gerrit hostname is required")
}

func TestProcessPubSubMessageInvalidFormat(t *testing.T) {
	t.Parallel()
	gs := &GerritSubscriber{}
	assert.ErrIsLike(t, gs.ProcessPubSubMessage(context.Background(), common.PubSubMessagePayload{
		Message: common.PubSubMessage{
			Data: "test",
		},
	}, "chromium", "invalid"), "message format \"invalid\" is not supported")
}

func convertEventToPubSubPayload(t *testing.T, event *gerritpb.SourceRepoEvent, format string) common.PubSubMessagePayload {
	t.Helper()
	switch format {
	case protoBinaryFormat:
		bin, err := proto.Marshal(event)
		assert.NoErr(t, err, truth.LineContext())
		return common.PubSubMessagePayload{
			Message: common.PubSubMessage{
				Data: base64.StdEncoding.EncodeToString(bin),
			},
		}
	case jsonFormat:
		json, err := protojson.Marshal(event)
		assert.NoErr(t, err, truth.LineContext())
		return common.PubSubMessagePayload{
			Message: common.PubSubMessage{
				Data: base64.StdEncoding.EncodeToString(json),
			},
		}
	default:
		t.Fatalf("unknown format %v", format)
	}
	return common.PubSubMessagePayload{}
}
