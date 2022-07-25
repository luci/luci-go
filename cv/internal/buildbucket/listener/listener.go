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

package bblistener

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	v1pb "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	pubsubutils "go.chromium.org/luci/cv/internal/common/pubsub"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const (
	// NumConcurrentListeners defines the number of Buildbucket Pubsub listeners
	// that runs concurrently.
	//
	// Increase this value if the notification processing speed can't keep up
	// with the incoming speed.
	NumConcurrentListeners = 5
	// SubscriptionID is the default subscription ID for listening to Buildbucket
	// build updates.
	SubscriptionID = "buildbucket-builds"
	// ListenDuration is how long each listener will running for.
	//
	// This should be in sync with the interval of the cron job that kicks the
	// listener to ensure continuous processing of Buildbucket pubsub events
	ListenDuration = 5 * time.Minute
)

// This interface encapsulate the communication with tryjob component.
type tryjobNotifier interface {
	ScheduleUpdate(context.Context, common.TryjobID, tryjob.ExternalID) error
}

// Register register tasks for listener and returns a function to kick off
// `NumConcurrentListeners` listeners.
func Register(tqd *tq.Dispatcher, projectID string, tjNotifier tryjobNotifier) func(context.Context) error {
	_ = tqd.RegisterTaskClass(tq.TaskClass{
		ID:           "listen-bb-pubsub",
		Prototype:    &ListenBBPubsubTask{},
		Queue:        "listen-bb-pubsub",
		Kind:         tq.NonTransactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*ListenBBPubsubTask)
			listener := &pubsubutils.PullingBatchProcessor{
				ProcessBatch: func(ctx context.Context, msgs []*pubsub.Message) error {
					return processNotificationsBatch(ctx, tjNotifier, notsFromMsgs(msgs))
				},
				ProjectID: projectID,
				SubID:     SubscriptionID,
				Options: pubsubutils.Options{
					ReceiveDuration: task.GetDuration().AsDuration(),
				},
			}
			if err := listener.Validate(); err != nil {
				return err
			}
			// Never retry the tasks because the listener will be started
			// periodically by the Cron.
			if err := listener.Process(ctx); err != nil {
				return common.TQIfy{NeverRetry: true}.Error(ctx, err)
			}
			return nil
		},
	})
	return func(ctx context.Context) error {
		return parallel.FanOutIn(func(workCh chan<- func() error) {
			for i := 0; i < NumConcurrentListeners; i++ {
				i := i
				workCh <- func() error {
					return tqd.AddTask(ctx, &tq.Task{
						Title: fmt.Sprintf("listener-%d", i),
						Payload: &ListenBBPubsubTask{
							Duration: durationpb.New(ListenDuration),
						},
					})
				}
			}
		})
	}
}

func processNotificationsBatch(ctx context.Context, tjNotifier tryjobNotifier, msgs []notification) error {
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
				switch err := tjNotifier.ScheduleUpdate(ctx, ids[i], eids[i]); {
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
