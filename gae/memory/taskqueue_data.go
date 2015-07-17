// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"net/http"
	"sync"
	"sync/atomic"

	"infra/gae/libs/gae"

	"github.com/luci/luci-go/common/clock"
)

var (
	currentNamespace = http.CanonicalHeaderKey("X-AppEngine-Current-Namespace")
	defaultNamespace = http.CanonicalHeaderKey("X-AppEngine-Default-Namespace")
)

//////////////////////////////// taskQueueData /////////////////////////////////

type taskQueueData struct {
	sync.Mutex

	named    gae.QueueData
	archived gae.QueueData
}

var (
	_ = memContextObj((*taskQueueData)(nil))
	_ = gae.TQTestable((*taskQueueData)(nil))
)

func newTaskQueueData() memContextObj {
	return &taskQueueData{
		named:    gae.QueueData{"default": {}},
		archived: gae.QueueData{"default": {}},
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
func (t *taskQueueData) mkTxn(*gae.DSTransactionOptions) memContextObj {
	return &txnTaskQueueData{
		parent: t,
		anony:  gae.AnonymousQueueData{},
	}
}

func (t *taskQueueData) GetTransactionTasks() gae.AnonymousQueueData {
	return nil
}

func (t *taskQueueData) CreateQueue(queueName string) {
	t.Lock()
	defer t.Unlock()

	if _, ok := t.named[queueName]; ok {
		panic(fmt.Errorf("memory/taskqueue: cannot add the same queue twice! %q", queueName))
	}
	t.named[queueName] = map[string]*gae.TQTask{}
	t.archived[queueName] = map[string]*gae.TQTask{}
}

func (t *taskQueueData) GetScheduledTasks() gae.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.named)
}

func (t *taskQueueData) GetTombstonedTasks() gae.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.archived)
}

func (t *taskQueueData) resetTasksWithLock() {
	for queueName := range t.named {
		t.named[queueName] = map[string]*gae.TQTask{}
		t.archived[queueName] = map[string]*gae.TQTask{}
	}
}

func (t *taskQueueData) ResetTasks() {
	t.Lock()
	defer t.Unlock()

	t.resetTasksWithLock()
}

func (t *taskQueueData) getQueueName(queueName string) (string, error) {
	if queueName == "" {
		queueName = "default"
	}
	if _, ok := t.named[queueName]; !ok {
		return "", errors.New("UNKNOWN_QUEUE")
	}
	return queueName, nil
}

var tqOkMethods = map[string]struct{}{
	"GET":    {},
	"POST":   {},
	"HEAD":   {},
	"PUT":    {},
	"DELETE": {},
}

func (t *taskQueueData) prepTask(c context.Context, ns string, task *gae.TQTask, queueName string) (*gae.TQTask, string, error) {
	queueName, err := t.getQueueName(queueName)
	if err != nil {
		return nil, "", err
	}

	toSched := dupTask(task)

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
		return nil, "", fmt.Errorf("taskqueue: bad method %q", toSched.Method)
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
			return nil, "", errors.New("INVALID_TASK_NAME")
		}
	}

	return toSched, queueName, nil
}

/////////////////////////////// txnTaskQueueData ///////////////////////////////

type txnTaskQueueData struct {
	lock sync.Mutex

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	anony  gae.AnonymousQueueData
	parent *taskQueueData
}

var (
	_ = memContextObj((*txnTaskQueueData)(nil))
	_ = gae.TQTestable((*txnTaskQueueData)(nil))
)

func (t *txnTaskQueueData) canApplyTxn(obj memContextObj) bool            { return false }
func (t *txnTaskQueueData) applyTxn(context.Context, memContextObj)       { panic("impossible") }
func (t *txnTaskQueueData) mkTxn(*gae.DSTransactionOptions) memContextObj { panic("impossible") }

func (t *txnTaskQueueData) endTxn() {
	if atomic.LoadInt32(&t.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&t.closed, 1)
}

func (t *txnTaskQueueData) run(f func() error) error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if atomic.LoadInt32(&t.closed) == 1 {
		return fmt.Errorf("taskqueue: transaction context has expired")
	}
	return f()
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

func (t *txnTaskQueueData) GetTransactionTasks() gae.AnonymousQueueData {
	t.Lock()
	defer t.Unlock()

	ret := make(gae.AnonymousQueueData, len(t.anony))
	for k, vs := range t.anony {
		ret[k] = make([]*gae.TQTask, len(vs))
		for i, v := range vs {
			tsk := dupTask(v)
			tsk.Name = ""
			ret[k][i] = tsk
		}
	}

	return ret
}

func (t *txnTaskQueueData) GetTombstonedTasks() gae.QueueData {
	return t.parent.GetTombstonedTasks()
}

func (t *txnTaskQueueData) GetScheduledTasks() gae.QueueData {
	return t.parent.GetScheduledTasks()
}

func (t *txnTaskQueueData) CreateQueue(queueName string) {
	t.parent.CreateQueue(queueName)
}
