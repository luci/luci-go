// Copyright 2021 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"encoding/json"
	"sync"

	"cloud.google.com/go/pubsub"

	v1pb "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	pubsubutils "go.chromium.org/luci/cv/internal/common/pubsub"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// This interface is implemented by tryjob.Updater, but can be mocked in this
// package for testing.
type scheduler interface {
	Schedule(context.Context, common.TryjobID, tryjob.ExternalID) error
}

// NewListener creates a pulling batch processor that pulls Buildbucket build
// notifications from the PubSub subscription specified by projectID and subID,
// and when they concern a Tryjob corresponding to one of our Runs, schedules
// tasks to notify the appropriate Run Manager.
func NewListener(updater scheduler, projectID, subID string) (*pubsubutils.PullingBatchProcessor, error) {
	listener := &pubsubutils.PullingBatchProcessor{
		ProcessBatch: func(ctx context.Context, msgs []*pubsub.Message) error {
			return processNotificationsBatch(ctx, updater, notsFromMsgs(msgs))
		},
		ProjectID: projectID,
		SubID:     subID,
	}
	if err := listener.Validate(); err != nil {
		return nil, err
	}
	return listener, nil
}

func processNotificationsBatch(ctx context.Context, updater scheduler, msgs []notification) error {
	var lastPermErr error

	eids := make([]tryjob.ExternalID, 0, len(msgs))
	remainingMessages := make([]notification, 0, len(msgs))
	for i := range msgs {
		eid, err := getBuildIDFromPubsubMessage(ctx, msgs[i].GetData())
		if err == nil {
			eids = append(eids, eid)
			remainingMessages = append(remainingMessages, msgs[i])
			continue
		}
		common.LogError(ctx, err)
		lastPermErr = err
		// Dismiss unparseable messages.
		msgs[i].Ack()
	}

	ids, err := tryjob.Resolve(ctx, eids...)
	if err != nil {
		return err
	}

	var lastPermErrMutex sync.Mutex
	poolErr := parallel.WorkPool(min(8, len(ids)), func(work chan<- func() error) {
		for i := range ids {
			i := i
			if ids[i] == 0 {
				// Dismiss irrelevant notifications.
				remainingMessages[i].Ack()
				continue
			}
			work <- func() error {
				switch err := updater.Schedule(ctx, ids[i], eids[i]); {
				case err == nil:
					remainingMessages[i].Ack()
				case transient.Tag.In(err):
					logging.Warningf(ctx, "transient error in scheduling update to %q: %s", eids[i], err)
					remainingMessages[i].Nack()
				default:
					common.LogError(ctx, err)
					lastPermErrMutex.Lock()
					defer lastPermErrMutex.Unlock()
					lastPermErr = err
					// Dismiss notifications that cause permanent errors.
					remainingMessages[i].Ack()
				}
				return nil
			}
		}
	})
	if poolErr != nil {
		panic(poolErr)
	}
	return lastPermErr
}

type buildMessage struct {
	Build    *v1pb.LegacyApiCommonBuildMessage `json:"build"`
	Hostname string                            `json:"hostname"`
}

func getBuildIDFromPubsubMessage(ctx context.Context, data []byte) (tryjob.ExternalID, error) {
	build := &buildMessage{}
	if err := json.Unmarshal(data, build); err != nil {
		return "", errors.Annotate(err, "while unmarshalling build notification").Err()
	}

	if build.Build == nil || build.Hostname == "" || build.Build.Id == 0 {
		return "", errors.Reason("missing build details in pubsub message: %s", data).Err()
	}
	return tryjob.BuildbucketID(build.Hostname, build.Build.Id)
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

type notification interface {
	Ack()
	Nack()
	GetData() []byte
}

type psNotification struct {
	*pubsub.Message
}

func (psn *psNotification) GetData() []byte {
	return psn.Data
}

func notsFromMsgs(msgs []*pubsub.Message) []notification {
	ret := make([]notification, 0, len(msgs))
	for _, msg := range msgs {
		ret = append(ret, &psNotification{msg})
	}
	return ret
}
