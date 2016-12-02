// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

// QueueData is {queueName: {taskName: *TQTask}}
type QueueData map[string]map[string]*Task

// AnonymousQueueData is {queueName: [*TQTask]}
type AnonymousQueueData map[string][]*Task

// Testable is the testable interface for fake taskqueue implementations
type Testable interface {
	CreateQueue(queueName string)
	CreatePullQueue(queueName string)
	GetScheduledTasks() QueueData
	GetTombstonedTasks() QueueData
	GetTransactionTasks() AnonymousQueueData
	ResetTasks()
}
