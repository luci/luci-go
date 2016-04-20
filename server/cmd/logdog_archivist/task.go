// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	gcps "google.golang.org/cloud/pubsub"
)

// pubsubArchiveTask implements the archivist.Task interface for a ArchiveTask
// Pub/Sub message.
type pubSubArchivistTask struct {
	// Context is a cloud package authenticated Context that can be used for raw
	// Pub/Sub interaction. This is necessary because ModifyAckDeadline is not
	// available to the "new API" Client.
	context.Context

	// subscriptionName is the name of the subscription that this task was pulled
	// from. This is NOT the full subscription path.
	subscriptionName string
	// msg is the message that this task is bound to.
	msg *gcps.Message

	// at is the unmarshalled ArchiveTask from msg.
	at logdog.ArchiveTask
}

func makePubSubArchivistTask(c context.Context, s string, msg *gcps.Message) (*pubSubArchivistTask, error) {
	// If we can't decode the archival task, we can't decide whether or not to
	// delete it, so we will leave it in the queue.
	t := pubSubArchivistTask{
		Context:          c,
		subscriptionName: s,
		msg:              msg,
	}

	if err := proto.Unmarshal(msg.Data, &t.at); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *pubSubArchivistTask) UniqueID() string {
	// The Message's AckID is guaranteed to be unique for a single lease.
	return t.msg.AckID
}

func (t *pubSubArchivistTask) Task() *logdog.ArchiveTask {
	return &t.at
}

func (t *pubSubArchivistTask) AssertLease(c context.Context) error {
	return retry.Retry(c, retry.Default, func() error {
		// Call ModifyAckDeadline directly, since we need immediate confirmation of
		// our continued ownership of the ACK. This will change the ACK's state
		// from that expected by the Message Iterator's keepalive system; however,
		// since we're extending it to the maximum deadline, worst-case the
		// keepalive will underestimate it and aggressively modify it.
		//
		// In practice, we tell the keepalive to use the maximum ACK deadline too,
		// so the disconnect will be minor at best.
		return gcps.ModifyAckDeadline(t, t.subscriptionName, t.msg.AckID, pubsub.MaxACKDeadline)
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(c, "Failed to modify ACK deadline. Retrying...")
	})
}
