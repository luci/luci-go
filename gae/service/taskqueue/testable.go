// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

// QueueData is {queueName: {taskName: *TQTask}}
type QueueData map[string]map[string]*Task

// AnonymousQueueData is {queueName: [*TQTask]}
type AnonymousQueueData map[string][]*Task

// Testable is the testable interface for fake taskqueue implementations
type Testable interface {
	CreateQueue(queueName string)
	GetScheduledTasks() QueueData
	GetTombstonedTasks() QueueData
	GetTransactionTasks() AnonymousQueueData
	ResetTasks()
}
