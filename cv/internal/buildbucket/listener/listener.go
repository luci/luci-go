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
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	bbv1pb "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const (
	// NumConcurrentListeners defines the number of Buildbucket Pub/Sub
	// listeners that run concurrently.
	//
	// Increase this value if the notification processing speed can't keep up
	// with the incoming speed.
	NumConcurrentListeners = 5
	// SubscriptionID is the default subscription ID for listening to
	// Buildbucket build updates.
	SubscriptionID = "buildbucket-builds"
	// ListenDuration is how long each listener will running for.
	//
	// This should be in sync with the interval of the cron job that kicks the
	// listener to ensure continuous processing of Buildbucket Pub/Sub events.
	ListenDuration = 5 * time.Minute
)

// This interface encapsulate the communication with tryjob component.
type tryjobNotifier interface {
	ScheduleUpdate(context.Context, common.TryjobID, tryjob.ExternalID) error
}

// Register registers tasks for listener and returns a function to kick off
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
			client, err := pubsub.NewClient(ctx, projectID)
			if err != nil {
				return err
			}
			defer func() {
				if err := client.Close(); err != nil {
					logging.Errorf(ctx, "failed to close PubSub client: %s", err)
				}
			}()
			l := &listener{
				pubsubClient: client,
				tjNotifier:   tjNotifier,
			}
			defer l.reportStats(ctx)
			duration := payload.(*ListenBBPubsubTask).GetDuration().AsDuration()
			if duration == 0 {
				duration = ListenDuration
			}
			cctx, cancel := clock.WithTimeout(ctx, duration)
			defer cancel()
			if err := l.start(cctx); err != nil {
				// Never retry the tasks because the listener will be started
				// periodically by the Cron.
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

type listener struct {
	pubsubClient *pubsub.Client
	tjNotifier   tryjobNotifier

	stats       listenerStats
	processedCh chan string // for testing only
}

type listenerStats struct {
	totalProcessedCount, transientErrCount, permanentErrCount int64
}

func (l *listener) start(ctx context.Context) error {
	subscription := l.pubsubClient.Subscription(SubscriptionID)
	return subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		switch err := l.processMsg(ctx, msg); {
		case err == nil:
			msg.Ack()
		case transient.Tag.In(err):
			logging.Warningf(ctx, "encounter transient error when processing buildbucket pubsub message %q; Reason: %s", string(msg.Data), err)
			msg.Nack()
			atomic.AddInt64(&l.stats.totalProcessedCount, 1)
		default:
			logging.Errorf(ctx, "encounter non-transient error when processing  buildbucket pubsub message: %q; Reason: %s", string(msg.Data), err)
			// Dismiss non-transient failure.
			msg.Ack()
			atomic.AddInt64(&l.stats.permanentErrCount, 1)
		}
		atomic.AddInt64(&l.stats.transientErrCount, 1)
		if l.processedCh != nil {
			l.processedCh <- msg.ID
		}
	})
}

func (l *listener) processMsg(ctx context.Context, msg *pubsub.Message) error {
	eid, err := parseExternalID(ctx, msg.Data)
	if err != nil {
		return err
	}
	switch ids, err := tryjob.Resolve(ctx, eid); {
	case err != nil:
		return err
	case len(ids) != 1:
		panic(fmt.Errorf("impossible; requested to resolve 1 external ID %s, got %d", eid, len(ids)))
	case ids[0] != 0:
		// Build that is tracked by LUCI CV.
		return l.tjNotifier.ScheduleUpdate(ctx, ids[0], eid)
	}
	return nil
}

type buildMessage struct {
	Build    *bbv1pb.LegacyApiCommonBuildMessage `json:"build"`
	Hostname string                              `json:"hostname"`
}

func parseExternalID(ctx context.Context, data []byte) (tryjob.ExternalID, error) {
	build := &buildMessage{}
	if err := json.Unmarshal(data, build); err != nil {
		return "", errors.Annotate(err, "while unmarshalling build notification").Err()
	}

	if build.Build == nil || build.Hostname == "" || build.Build.Id == 0 {
		return "", errors.Reason("missing build details in pubsub message: %s", data).Err()
	}
	return tryjob.BuildbucketID(build.Hostname, build.Build.Id)
}

func (l *listener) reportStats(ctx context.Context) {
	logging.Infof(ctx, "processed %d buildbucket pubsub messages in total. %d of them have transient failure. %d of them have non-transient failure",
		l.stats.totalProcessedCount,
		l.stats.transientErrCount,
		l.stats.permanentErrCount)
	// TODO(yiwzhang): send tsmon metrics. Especially for non-transient count to
	// to alert on.
}
