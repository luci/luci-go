// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
)

var (
	currentNamespace = http.CanonicalHeaderKey("X-AppEngine-Current-Namespace")
	defaultNamespace = http.CanonicalHeaderKey("X-AppEngine-Default-Namespace")
)

//////////////////////////////// taskQueueData /////////////////////////////////

type taskQueueData struct {
	sync.Mutex

	named    tq.QueueData
	archived tq.QueueData
}

var (
	_ = memContextObj((*taskQueueData)(nil))
	_ = tq.Testable((*taskQueueData)(nil))
)

func newTaskQueueData() memContextObj {
	return &taskQueueData{
		named:    tq.QueueData{"default": {}},
		archived: tq.QueueData{"default": {}},
	}
}

func (t *taskQueueData) canApplyTxn(obj memContextObj) bool { return true }
func (t *taskQueueData) endTxn()                            {}
func (t *taskQueueData) applyTxn(c context.Context, obj memContextObj) {
	txn := obj.(*txnTaskQueueData)
	for qn, tasks := range txn.anony {
		for _, tsk := range tasks {
			tsk.Name = mkName(c, tsk.Name, t.named[qn])
			t.named[qn][tsk.Name] = tsk
		}
	}
	txn.anony = nil
}
func (t *taskQueueData) mkTxn(*ds.TransactionOptions) memContextObj {
	return &txnTaskQueueData{
		parent: t,
		anony:  tq.AnonymousQueueData{},
	}
}

func (t *taskQueueData) GetTransactionTasks() tq.AnonymousQueueData {
	return nil
}

func (t *taskQueueData) CreateQueue(queueName string) {
	t.Lock()
	defer t.Unlock()

	if _, ok := t.named[queueName]; ok {
		panic(fmt.Errorf("memory/taskqueue: cannot add the same queue twice! %q", queueName))
	}
	t.named[queueName] = map[string]*tq.Task{}
	t.archived[queueName] = map[string]*tq.Task{}
}

func (t *taskQueueData) GetScheduledTasks() tq.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.named)
}

func (t *taskQueueData) GetTombstonedTasks() tq.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.archived)
}

func (t *taskQueueData) resetTasksWithLock() {
	for queueName := range t.named {
		t.named[queueName] = map[string]*tq.Task{}
		t.archived[queueName] = map[string]*tq.Task{}
	}
}

func (t *taskQueueData) ResetTasks() {
	t.Lock()
	defer t.Unlock()

	t.resetTasksWithLock()
}

func (t *taskQueueData) getQueueNameLocked(queueName string) (string, error) {
	if queueName == "" {
		queueName = "default"
	}
	if _, ok := t.named[queueName]; !ok {
		return "", errors.New("UNKNOWN_QUEUE")
	}
	return queueName, nil
}

func (t *taskQueueData) purgeLocked(queueName string) error {
	queueName, err := t.getQueueNameLocked(queueName)
	if err != nil {
		return err
	}

	t.named[queueName] = map[string]*tq.Task{}
	t.archived[queueName] = map[string]*tq.Task{}
	return nil
}

var tqOkMethods = map[string]struct{}{
	"GET":    {},
	"POST":   {},
	"HEAD":   {},
	"PUT":    {},
	"DELETE": {},
}

func (t *taskQueueData) prepTask(c context.Context, ns string, task *tq.Task, queueName string) (*tq.Task, error) {
	toSched := task.Duplicate()

	if toSched.Path == "" {
		toSched.Path = "/_ah/queue/" + queueName
	}

	if toSched.ETA.IsZero() {
		toSched.ETA = clock.Now(c).Add(toSched.Delay)
	} else if toSched.Delay != 0 {
		panic("taskqueue: both Delay and ETA are set")
	}
	toSched.Delay = 0

	if toSched.Method == "" {
		toSched.Method = "POST"
	}
	if _, ok := tqOkMethods[toSched.Method]; !ok {
		return nil, fmt.Errorf("taskqueue: bad method %q", toSched.Method)
	}
	if toSched.Method != "POST" && toSched.Method != "PUT" {
		toSched.Payload = nil
	}

	if _, ok := toSched.Header[currentNamespace]; !ok {
		if ns != "" {
			if toSched.Header == nil {
				toSched.Header = http.Header{}
			}
			toSched.Header[currentNamespace] = []string{ns}
		}
	}
	// TODO(riannucci): implement DefaultNamespace

	if toSched.Name == "" {
		toSched.Name = mkName(c, "", t.named[queueName])
	} else {
		if !validTaskName.MatchString(toSched.Name) {
			return nil, errors.New("INVALID_TASK_NAME")
		}
	}

	return toSched, nil
}

/////////////////////////////// txnTaskQueueData ///////////////////////////////

type txnTaskQueueData struct {
	lock sync.Mutex

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	anony  tq.AnonymousQueueData
	parent *taskQueueData
}

var (
	_ = memContextObj((*txnTaskQueueData)(nil))
	_ = tq.Testable((*txnTaskQueueData)(nil))
)

func (t *txnTaskQueueData) canApplyTxn(obj memContextObj) bool         { return false }
func (t *txnTaskQueueData) applyTxn(context.Context, memContextObj)    { panic("impossible") }
func (t *txnTaskQueueData) mkTxn(*ds.TransactionOptions) memContextObj { panic("impossible") }

func (t *txnTaskQueueData) endTxn() {
	if atomic.LoadInt32(&t.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&t.closed, 1)
}

func (t *txnTaskQueueData) ResetTasks() {
	t.Lock()
	defer t.Unlock()

	for queuename := range t.anony {
		t.anony[queuename] = nil
	}
	t.parent.resetTasksWithLock()
}

func (t *txnTaskQueueData) Lock() {
	t.lock.Lock()
	t.parent.Lock()
}
func (t *txnTaskQueueData) Unlock() {
	t.parent.Unlock()
	t.lock.Unlock()
}

func (t *txnTaskQueueData) GetTransactionTasks() tq.AnonymousQueueData {
	t.Lock()
	defer t.Unlock()

	ret := make(tq.AnonymousQueueData, len(t.anony))
	for k, vs := range t.anony {
		ret[k] = make([]*tq.Task, len(vs))
		for i, v := range vs {
			tsk := v.Duplicate()
			tsk.Name = ""
			ret[k][i] = tsk
		}
	}

	return ret
}

func (t *txnTaskQueueData) GetTombstonedTasks() tq.QueueData {
	return t.parent.GetTombstonedTasks()
}

func (t *txnTaskQueueData) GetScheduledTasks() tq.QueueData {
	return t.parent.GetScheduledTasks()
}

func (t *txnTaskQueueData) CreateQueue(queueName string) {
	t.parent.CreateQueue(queueName)
}
