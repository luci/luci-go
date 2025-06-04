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

// Package subscriber implements the logic to process Gerrit pubsub messages.
package subscriber

import (
	"context"
	"encoding/base64"
	"regexp"
	"strconv"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/caching"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

// This interface encapsulates the communication with changelist.Updater.
type clUpdater interface {
	Schedule(context.Context, *changelist.UpdateCLTask) error
}

// GerritSubscriber processes pubsub messages from Gerrit.
type GerritSubscriber struct {
	// CLUpdater is used to schedule CL update tasks.
	CLUpdater clUpdater
}

const (
	jsonFormat        = "json"
	protoBinaryFormat = "binary"
)

// ProcessPubSubMessage processes a pub/sub message from Gerrit.
//
// It schedules UpdateCLTask(s) for all the LUCI projects watching the CL
// in the pub/sub message.
func (gs *GerritSubscriber) ProcessPubSubMessage(ctx context.Context, payload common.PubSubMessagePayload, hostName string, format string) error {
	switch {
	case hostName == "":
		return errors.New("gerrit hostname is required")
	case len(payload.Message.Data) == 0:
		return nil
	}

	data, err := base64.StdEncoding.DecodeString(payload.Message.Data)
	if err != nil {
		return errors.Fmt("failed to decode message data: %w", err)
	}
	event := &gerritpb.SourceRepoEvent{}
	switch format {
	case jsonFormat:
		if err := protojson.Unmarshal(data, event); err != nil {
			return errors.Fmt("failed to unmarshal proto json data: %w", err)
		}
	case protoBinaryFormat:
		if err := proto.Unmarshal(data, event); err != nil {
			return errors.Fmt("failed to unmarshal proto binary data: %w", err)
		}
	default:
		return errors.Fmt("message format %q is not supported", format)
	}

	eidToMetaRevID, err := computeExternalIDToMetaRevID(ctx, event, hostName)
	switch {
	case err != nil:
		return err
	case len(eidToMetaRevID) == 0:
		return nil
	}

	repo := extractRepoFromEventName(event.Name)
	if repo == "" {
		return errors.Fmt("failed to extract repo from event name %q", event.Name)
	}

	luciProjects, err := findLUCIProjectsToNotify(ctx, hostName, repo)
	if err != nil {
		return err
	}
	return parallel.FanOutIn(func(workCh chan<- func() error) {
		for eid, meta := range eidToMetaRevID {
			for _, project := range luciProjects {
				task := &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(eid),
					Requester:   changelist.UpdateCLTask_PUBSUB_PUSH,
					Hint:        &changelist.UpdateCLTask_Hint{MetaRevId: meta},
				}
				workCh <- func() error {
					if err := gs.CLUpdater.Schedule(ctx, task); err != nil {
						return errors.Fmt("failed to schedule CL update task: %w", err)
					}
					return nil
				}
			}
		}
	})
}

var refNameRE = regexp.MustCompile(`refs/changes/\d+/(\d+)/meta`)

func computeExternalIDToMetaRevID(ctx context.Context, event *gerritpb.SourceRepoEvent, host string) (map[changelist.ExternalID]string, error) {
	if event.GetRefUpdateEvent() == nil {
		return nil, nil
	}
	ret := make(map[changelist.ExternalID]string)
	for ref, ev := range event.GetRefUpdateEvent().GetRefUpdates() {
		// LUCI CV is only interested in CL update events, of which ref name
		// ends with "/meta" in the following format.
		// : "refs/changes/<val>/<change_num>/meta"
		subMatches := refNameRE.FindStringSubmatch(ref)
		if len(subMatches) < 2 {
			continue
		}
		change, err := strconv.ParseInt(subMatches[1], 10, 63)
		if err != nil {
			// Must be a bug either in Gerrit or LUCI CV.
			return nil, errors.Fmt("invalid change num (%s): %s: %w", subMatches[1], event, err)
		}
		eid, err := changelist.GobID(host, change)
		if err != nil {
			return nil, err
		}

		switch prev, exist := ret[eid]; {
		case exist && prev != ev.NewId:
			// RefUpdateEvent is a map type. Therefore, a single pubsub
			// message can have at most one update event for each of the CLs
			// listed.
			//
			// If a duplicate ExternalID with different RevID is found,
			// there is a bug in CV or Gerrit.
			return nil, errors.Fmt("found multiple meta-rev-ids (%q, %q) for %q: %s",
				prev, ev.NewId, eid, event)
		case exist && prev == ev.NewId:
			// Still strange, but ok.
			logging.Warningf(ctx, "duplicate update events found for %q: %s", eid, event)
		case !exist:
			logging.Infof(ctx, "Received update event for CL %s", string(eid))
			ret[eid] = ev.NewId
		}
	}
	return ret, nil
}

var eventNameRe = regexp.MustCompile(`projects/[^/]+/repos/(.+)`)

func extractRepoFromEventName(eventName string) string {
	subMatches := eventNameRe.FindStringSubmatch(eventName)
	if len(subMatches) < 2 {
		return ""
	}
	return subMatches[1]
}

// listenerTypeCaches caches the type of Gerrit listener for up to 200
// LUCI projects.
var listenerTypeCaches = caching.RegisterLRUCache[string, cfgpb.Config_GerritListenerType](200)

// look for projects that watch the provided host+repo and use pubsub as Gerrit
// listener type.
func findLUCIProjectsToNotify(ctx context.Context, host, repo string) ([]string, error) {
	projects, err := gobmap.LookupProjects(ctx, host, repo)
	if err != nil {
		return nil, err
	}
	var ret []string
	for _, project := range projects {
		cache := listenerTypeCaches.LRU(ctx)
		if cache == nil {
			return nil, errors.New("process cache data is missing")
		}
		listenerType, err := cache.GetOrCreate(ctx, project, func() (v cfgpb.Config_GerritListenerType, exp time.Duration, err error) {
			meta, err := prjcfg.GetLatestMeta(ctx, project)
			if err != nil {
				return cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB, 0, err
			}
			if len(meta.ConfigGroupIDs) == 0 {
				return cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB, 0, errors.Fmt("project %q doesn't have any config group", project)
			}
			// gerrit_listener_type is a project level field so it will be the
			// same for all config group. Pick the first config group here.
			cg, err := prjcfg.GetConfigGroup(ctx, project, meta.ConfigGroupIDs[0])
			if err != nil {
				return cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB, 0, err
			}
			return cg.GerritListenerType, 5 * time.Minute, nil
		})
		if err != nil {
			return nil, err
		}
		switch listenerType {
		case cfgpb.Config_GERRIT_LISTENER_TYPE_UNSPECIFIED:
			// TODO: crbug/421431364: error once all project configs have explicitly
			// set pubsub as listener type.
			fallthrough
		case cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB:
			ret = append(ret, project)
		case cfgpb.Config_GERRIT_LISTENER_TYPE_LEGACY_POLLER: // skip the poller type
		default:
			return nil, errors.Fmt("gerrit listener type %s is not supported", listenerType)
		}
	}
	return ret, nil
}
