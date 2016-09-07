// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"strconv"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/services/v1"

	gcps "cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// pubsubArchiveTask implements the archivist.Task interface for a ArchiveTask
// Pub/Sub message.
type pubSubArchivistTask struct {
	// subscriptionName is the name of the subscription that this task was pulled
	// from. This is NOT the full subscription path.
	subscriptionName string
	// msg is the message that this task is bound to.
	msg *gcps.Message

	// timestamp is the time when this message was received.
	timestamp time.Time

	// at is the unmarshalled ArchiveTask from msg.
	at logdog.ArchiveTask

	// consumed is true if this task has been marked for consumption.
	consumed bool
}

func makePubSubArchivistTask(c context.Context, s string, msg *gcps.Message) (*pubSubArchivistTask, error) {
	// If we can't decode the archival task, we can't decide whether or not to
	// delete it, so we will leave it in the queue.
	t := pubSubArchivistTask{
		subscriptionName: s,
		msg:              msg,
		timestamp:        clock.Now(c),
	}

	if err := proto.Unmarshal(msg.Data, &t.at); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *pubSubArchivistTask) UniqueID() string {
	// Use the message's reception timestamp as its unique identifier. We
	// represent this as a hexadecimal-formatted seconds-since-epoch value.
	return strconv.FormatInt(t.timestamp.Unix(), 16)
}

func (t *pubSubArchivistTask) Task() *logdog.ArchiveTask {
	return &t.at
}

func (t *pubSubArchivistTask) Consume() {
	t.consumed = true
}

func (t *pubSubArchivistTask) AssertLease(c context.Context) error { return nil }
