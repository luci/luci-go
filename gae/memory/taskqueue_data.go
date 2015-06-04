// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"infra/gae/libs/wrapper"
	"net/http"
	"sync"
	"sync/atomic"

	"appengine/datastore"
	"appengine/taskqueue"
	pb "appengine_internal/taskqueue"
	"golang.org/x/net/context"
	"infra/libs/clock"
)

var (
	currentNamespace = http.CanonicalHeaderKey("X-AppEngine-Current-Namespace")
	defaultNamespace = http.CanonicalHeaderKey("X-AppEngine-Default-Namespace")
)

//////////////////////////////// taskQueueData /////////////////////////////////

type taskQueueData struct {
	sync.Mutex
	wrapper.BrokenFeatures

	named    wrapper.QueueData
	archived wrapper.QueueData
}

var (
	_ = memContextObj((*taskQueueData)(nil))
	_ = wrapper.TQTestable((*taskQueueData)(nil))
)

func newTaskQueueData() memContextObj {
	return &taskQueueData{
		BrokenFeatures: wrapper.BrokenFeatures{
			DefaultError: newTQError(pb.TaskQueueServiceError_TRANSIENT_ERROR)},
		named:    wrapper.QueueData{"default": {}},
		archived: wrapper.QueueData{"default": {}},
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
func (t *taskQueueData) mkTxn(*datastore.TransactionOptions) (memContextObj, error) {
	return &txnTaskQueueData{
		BrokenFeatures: &t.BrokenFeatures,
		parent:         t,
		anony:          wrapper.AnonymousQueueData{},
	}, nil
}

func (t *taskQueueData) GetTransactionTasks() wrapper.AnonymousQueueData {
	return nil
}

func (t *taskQueueData) CreateQueue(queueName string) {
	t.Lock()
	defer t.Unlock()

	if _, ok := t.named[queueName]; ok {
		panic(fmt.Errorf("memory/taskqueue: cannot add the same queue twice! %q", queueName))
	}
	t.named[queueName] = map[string]*taskqueue.Task{}
	t.archived[queueName] = map[string]*taskqueue.Task{}
}

func (t *taskQueueData) GetScheduledTasks() wrapper.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.named)
}

func (t *taskQueueData) GetTombstonedTasks() wrapper.QueueData {
	t.Lock()
	defer t.Unlock()

	return dupQueue(t.archived)
}

func (t *taskQueueData) resetTasksWithLock() {
	for queuename := range t.named {
		t.named[queuename] = map[string]*taskqueue.Task{}
		t.archived[queuename] = map[string]*taskqueue.Task{}
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
		return "", newTQError(pb.TaskQueueServiceError_UNKNOWN_QUEUE)
	}
	return queueName, nil
}

func (t *taskQueueData) prepTask(c context.Context, ns string, task *taskqueue.Task, queueName string) (
	*taskqueue.Task, string, error) {
	queueName, err := t.getQueueName(queueName)
	if err != nil {
		return nil, "", err
	}

	toSched := dupTask(task)

	if toSched.Path == "" {
		return nil, "", newTQError(pb.TaskQueueServiceError_INVALID_URL)
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
	if _, ok := pb.TaskQueueAddRequest_RequestMethod_value[toSched.Method]; !ok {
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
			return nil, "", newTQError(pb.TaskQueueServiceError_INVALID_TASK_NAME)
		}
	}

	return toSched, queueName, nil
}

/////////////////////////////// txnTaskQueueData ///////////////////////////////

type txnTaskQueueData struct {
	*wrapper.BrokenFeatures

	lock sync.Mutex

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	anony  wrapper.AnonymousQueueData
	parent *taskQueueData
}

var (
	_ = memContextObj((*txnTaskQueueData)(nil))
	_ = wrapper.TQTestable((*txnTaskQueueData)(nil))
)

func (t *txnTaskQueueData) canApplyTxn(obj memContextObj) bool { return false }

func (t *txnTaskQueueData) applyTxn(context.Context, memContextObj) {
	panic(errors.New("txnTaskQueueData.applyTxn is not implemented"))
}

func (t *txnTaskQueueData) mkTxn(*datastore.TransactionOptions) (memContextObj, error) {
	return nil, errors.New("txnTaskQueueData.mkTxn is not implemented")
}

func (t *txnTaskQueueData) endTxn() {
	if atomic.LoadInt32(&t.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&t.closed, 1)
}

func (t *txnTaskQueueData) IsBroken() error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if atomic.LoadInt32(&t.closed) == 1 {
		return fmt.Errorf("taskqueue: transaction context has expired")
	}
	return t.parent.IsBroken()
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

func (t *txnTaskQueueData) GetTransactionTasks() wrapper.AnonymousQueueData {
	t.Lock()
	defer t.Unlock()

	ret := make(wrapper.AnonymousQueueData, len(t.anony))
	for k, vs := range t.anony {
		ret[k] = make([]*taskqueue.Task, len(vs))
		for i, v := range vs {
			tsk := dupTask(v)
			tsk.Name = ""
			ret[k][i] = tsk
		}
	}

	return ret
}

func (t *txnTaskQueueData) GetTombstonedTasks() wrapper.QueueData {
	return t.parent.GetTombstonedTasks()
}

func (t *txnTaskQueueData) GetScheduledTasks() wrapper.QueueData {
	return t.parent.GetScheduledTasks()
}

func (t *txnTaskQueueData) CreateQueue(queueName string) {
	t.parent.CreateQueue(queueName)
}
