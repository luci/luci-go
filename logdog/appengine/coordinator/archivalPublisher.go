// Copyright 2016 The LUCI Authors.
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

package coordinator

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	gcps "cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"
)

// ArchivalPublisher is capable of publishing archival requests.
type ArchivalPublisher interface {
	// Close shutdowns this publisher instance, releasing all its resources.
	Close() error

	// Publish publishes the supplied ArchiveTask.
	Publish(context.Context, *logdog.ArchiveTask) error

	// NewPublishIndex returns a new publish index. Each publish index is unique
	// within its request.
	NewPublishIndex() uint64
}

// PubsubArchivalPublisher is an ArchivalPublisher that uses Cloud Pub/Sub
// for task archival.
type PubsubArchivalPublisher struct {
	// Publisher is the client used to publish messages.
	Publisher pubsub.Publisher

	// AECtx is the AppEngine Context to use for publish operations.
	AECtx context.Context

	// PublishIndexFunc is a function that will return a unique publish index
	// for this request.
	PublishIndexFunc func() uint64
}

// Close implements ArchivalPublisher.
func (p *PubsubArchivalPublisher) Close() error { return nil }

// Publish implements ArchivalPublisher.
func (p *PubsubArchivalPublisher) Publish(c context.Context, t *logdog.ArchiveTask) error {
	d, err := proto.Marshal(t)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to marshal task.")
		return err
	}

	msg := gcps.Message{
		Data: d,
	}

	// We want to bind our cancellation to "c", but need to use our AppEngine
	// Context for the actual Publish call. Handle this with a goroutine that will
	// cancel our AppEngine Context if "c" is cancelled.
	//
	// The goroutine will naturally exit at the end of this function when "AECtx"
	// is cancelled via defer.
	//
	// Publishing usually happens immediately.
	// If it's taken more than 15s, something has already gone horribly wrong,
	// so just kill it and try again.
	aeCtx, cancelFunc := clock.WithTimeout(p.AECtx, time.Second*15)
	defer cancelFunc()

	go func(aeCtx context.Context) {
		select {
		case <-aeCtx.Done():
		case <-c.Done():
			cancelFunc()
		}
	}(aeCtx)

	// Create a new AppEngine context. Don't pass gRPC metadata to PubSub, since
	// we don't want any caller RPC to be forwarded to the backend service.
	aeCtx = metadata.NewOutgoingContext(aeCtx, nil)

	log.Fields{
		"project": t.Project,
		"hash":    t.Id,
		"key":     t.Key,
	}.Infof(c, "Publishing archival message for stream.")

	_, err = p.Publisher.Publish(aeCtx, &msg)
	return err
}

// NewPublishIndex implements ArchivalPublisher.
func (p *PubsubArchivalPublisher) NewPublishIndex() uint64 {
	return p.PublishIndexFunc()
}
