// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

// QueueData is {queueName: {taskName: *TQTask}}
type QueueData map[string]map[string]*TQTask

// AnonymousQueueData is {queueName: [*TQTask]}
type AnonymousQueueData map[string][]*TQTask

// TQTestable is the testable interface for fake taskqueue implementations
type TQTestable interface {
	CreateQueue(queueName string)
	GetScheduledTasks() QueueData
	GetTombstonedTasks() QueueData
	GetTransactionTasks() AnonymousQueueData
	ResetTasks()
}
