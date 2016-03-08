// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueueClient

import (
	"encoding/base64"
	"time"

	"google.golang.org/api/taskqueue/v1beta2"
)

// Task is a Go-friendly wrapper around the task queue API Task structure.
type Task struct {
	// ID is the Task's ID.
	ID string
	// Queue is the name of the Task's queue.
	Queue string

	// EnqueueTime is the time when the task was enqueued.
	EnqueueTime time.Time
	// LeaseExpireTime is the time when the Task's lease will expire.
	LeaseExpireTime time.Time
	// RetryCount is the number of leases applied to this task.
	RetryCount int64

	// Payload is the Task's data payload.
	Payload []byte
}

func buildTask(t *taskqueue.Task) (*Task, error) {
	payload, err := base64.StdEncoding.DecodeString(t.PayloadBase64)
	if err != nil {
		return nil, err
	}

	return &Task{
		ID:              t.Id,
		Queue:           t.QueueName,
		EnqueueTime:     time.Unix(t.EnqueueTimestamp, 0),
		LeaseExpireTime: time.Unix(t.LeaseTimestamp, 0),
		RetryCount:      t.RetryCount,
		Payload:         payload,
	}, nil
}
