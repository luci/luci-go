// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"appengine/taskqueue"
)

// QueueData is {queueName: {taskName: *Task}}
type QueueData map[string]map[string]*taskqueue.Task

// AnonymousQueueData is {queueName: [*Task]}
type AnonymousQueueData map[string][]*taskqueue.Task

// TQTestable is the testable interface for fake taskqueue implementations
type TQTestable interface {
	Testable

	CreateQueue(queueName string)
	GetScheduledTasks() QueueData
	GetTombstonedTasks() QueueData
	GetTransactionTasks() AnonymousQueueData
	ResetTasks()
}
