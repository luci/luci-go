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
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/buildbucket"
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

// topicNameRegexp - the Cloud project Pub/Sub topic name regex expression.
var topicNameRegexp = regexp.MustCompile(`^projects/(.*)/topics/(.*)$`)

// This interface encapsulate the communication with tryjob component.
type tryjobNotifier interface {
	ScheduleUpdate(context.Context, common.TryjobID, tryjob.ExternalID) error
}

type tryjobUpdater interface {
	// Update updates the Tryjob entity associated with the given `eid`.
	//
	// `data` should contain the latest information of the Tryjob from the
	// Tryjob backend system (e.g. Build proto from Buildbucket pubsub).
	//
	// No-op if the Tryjob data stored in CV appears to be newer than the provided
	// data (e.g. has newer Tryjob.Result.UpdateTime)
	Update(ctx context.Context, eid tryjob.ExternalID, data any) error
}

// Register registers tasks for listener and returns a function to kick off
// `NumConcurrentListeners` listeners.
func Register(tqd *tq.Dispatcher, projectID string, tjNotifier tryjobNotifier, tjUpdater tryjobUpdater) func(context.Context) error {
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

			sub := client.Subscription(SubscriptionID)
			subConfig, err := sub.Config(ctx)
			if err != nil {
				return errors.Annotate(err, "failed to get configuration for the subscription %s", sub.String()).Err()
			}
			subscribedProj, err := extractTopicProject(subConfig.Topic.String())
			if err != nil {
				return errors.Annotate(err, "for subscription %s", sub.String()).Err()
			}
			l := &listener{
				bbHost:       fmt.Sprintf("%s.appspot.com", subscribedProj),
				subscription: sub,
				tjNotifier:   tjNotifier,
				tjUpdater:    tjUpdater,
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

// StartListenerForTest starts a buildbucket listener for testing purpose.
//
// Returns a callback function to stop the listener.
func StartListenerForTest(ctx context.Context, sub *pubsub.Subscription, tjNotifier tryjobNotifier) func() {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		l := &listener{
			subscription: sub,
			tjNotifier:   tjNotifier,
		}
		if err := l.start(cctx); err != nil {
			logging.Errorf(ctx, "encounter error in buildbucket listener: %s", err)
		}
	}()
	return cancel
}

type listener struct {
	bbHost       string // Buildbucket host that the subscription subscribes to. e.g. cr-buildbucket.appspot.com
	subscription *pubsub.Subscription
	tjNotifier   tryjobNotifier
	tjUpdater    tryjobUpdater

	stats       listenerStats
	processedCh chan string // for testing only
}

type listenerStats struct {
	totalProcessedCount, transientErrCount, permanentErrCount int64
}

func (l *listener) start(ctx context.Context) error {
	return l.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
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
	var isV2Msg bool
	var buildID int64
	var hostname string
	var build *buildbucketpb.Build
	var err error
	if v, ok := msg.Attributes["version"]; ok && v == "v2" {
		isV2Msg = true
		if build, err = parseV2Data(msg); err != nil {
			return err
		}
		hostname, buildID = build.GetInfra().GetBuildbucket().GetHostname(), build.GetId()
	} else {
		// TODO(crbug.com/1406393): delete it once the migration is done. And the
		// above pre-declared variables can also be deleted to make code more clean.
		parsedMsg, err := parseV1Data(ctx, msg.Data)
		if err != nil {
			return err
		}
		hostname, buildID = parsedMsg.Hostname, parsedMsg.Build.ID
	}

	if hostname == "" {
		logging.Warningf(ctx, "received pubsub message with empty hostname for build %d, using the computed one %s", build.GetId(), l.bbHost)
		hostname = l.bbHost
	}
	eid, err := tryjob.BuildbucketID(hostname, buildID)
	if err != nil {
		return err
	}
	switch id, err := eid.Resolve(ctx); {
	case err != nil:
		return err
	case id != 0:
		// Build is tracked by LUCI CV.
		if !isV2Msg {
			return l.tjNotifier.ScheduleUpdate(ctx, id, eid)
		}
		// TODO(crbug.com/1406393): remove the debugging once the migration is done.
		logging.Debugf(ctx, "builds_v2 pubsub listener: updating tryjob %s", eid)
		return l.tjUpdater.Update(ctx, eid, build)
	}
	return nil
}

// parseV2Data parses Buildbucket new `builds_v2` pubsub message data.
func parseV2Data(msg *pubsub.Message) (*buildbucketpb.Build, error) {
	buildsV2Msg := &buildbucketpb.BuildsV2PubSub{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(msg.Data, buildsV2Msg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal pubsub message into BuildsV2PubSub proto").Err()
	}
	largeFieldsData, err := zlibDecompress(buildsV2Msg.BuildLargeFields)
	if err != nil {
		return nil, errors.Annotate(err, "failed to decompress build_large_fields for build %d", buildsV2Msg.Build.GetId()).Err()
	}
	largeFields := &buildbucketpb.Build{}
	if err := (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(largeFieldsData, largeFields); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal build_large_fields for build %d", buildsV2Msg.Build.GetId()).Err()
	}
	proto.Merge(buildsV2Msg.Build, largeFields)

	return buildsV2Msg.Build, nil
}

// parseV1Data extracts the relevant information from Buildbucket old `builds`
// topic Pub/Sub message data.
func parseV1Data(ctx context.Context, data []byte) (buildbucket.PubsubMessage, error) {
	message := buildbucket.PubsubMessage{}
	// Extra fields that are not in the struct are ignored by json.Unmarshal.
	if err := json.Unmarshal(data, &message); err != nil {
		return buildbucket.PubsubMessage{}, errors.Annotate(err, "while unmarshalling build pubsub message").Err()
	}
	if message.Hostname == "" || message.Build.ID == 0 {
		return buildbucket.PubsubMessage{}, errors.Reason("missing build details in pubsub message: %s", data).Err()
	}
	return message, nil
}

func (l *listener) reportStats(ctx context.Context) {
	logging.Infof(ctx, "processed %d buildbucket pubsub messages in total. %d of them have transient failure. %d of them have non-transient failure",
		l.stats.totalProcessedCount,
		l.stats.transientErrCount,
		l.stats.permanentErrCount)
	// TODO(yiwzhang): send tsmon metrics. Especially for non-transient count to
	// to alert on.
}

// zlibDecompress decompresses data using zlib.
func zlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// extractTopicProject extracts the subscribed project name from the given subscription.
func extractTopicProject(topic string) (string, error) {
	matches := topicNameRegexp.FindStringSubmatch(topic)
	if len(matches) != 3 {
		return "", errors.Reason("topic %s doesn't match %q", topic, topicNameRegexp.String()).Err()
	}
	return matches[1], nil
}
