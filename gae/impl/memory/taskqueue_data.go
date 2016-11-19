// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/common/data/rand/mathrand"
)

var (
	currentNamespace = http.CanonicalHeaderKey("X-AppEngine-Current-Namespace")
	defaultNamespace = http.CanonicalHeaderKey("X-AppEngine-Default-Namespace")

	validTaskName = regexp.MustCompile("^[0-9a-zA-Z\\-\\_]{0,500}$")

	errBadRequest      = errors.New("BAD_REQUEST")
	errInvalidTaskName = errors.New("INVALID_TASK_NAME")
	errUnknownQueue    = errors.New("UNKNOWN_QUEUE")
	errTombstonedTask  = errors.New("TOMBSTONED_TASK")
	errUnknownTask     = errors.New("UNKNOWN_TASK")
)

//////////////////////////////// sortedQueue ///////////////////////////////////

type sortedQueue struct {
	name string

	tasks    map[string]*tq.Task // added, but not deleted
	archived map[string]*tq.Task // tombstones

	// TODO(vadimsh): add a structure sorted by ETA
}

func newSortedQueue(name string) *sortedQueue {
	return &sortedQueue{
		name:     name,
		tasks:    map[string]*tq.Task{},
		archived: map[string]*tq.Task{},
	}
}

// All sortedQueue methods are assumed to be called under taskQueueData lock.

func (q *sortedQueue) genTaskName(c context.Context) string {
	const validTaskChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"
	for {
		buf := [500]byte{}
		for i := 0; i < 500; i++ {
			buf[i] = validTaskChars[mathrand.Intn(c, len(validTaskChars))]
		}
		name := string(buf[:])
		_, ok1 := q.tasks[name]
		_, ok2 := q.archived[name]
		if !ok1 && !ok2 {
			return name
		}
	}
}

func (q *sortedQueue) addTask(task *tq.Task) error {
	if _, ok := q.archived[task.Name]; ok {
		// SDK converts TOMBSTONE -> already added too
		return tq.ErrTaskAlreadyAdded
	} else if _, ok := q.tasks[task.Name]; ok {
		return tq.ErrTaskAlreadyAdded
	}

	q.tasks[task.Name] = task

	// TODO(vadimsh): If a PULL task, add to ETA-sorted array.

	return nil
}

func (q *sortedQueue) deleteTask(task *tq.Task) error {
	if _, ok := q.archived[task.Name]; ok {
		return errTombstonedTask
	}

	if _, ok := q.tasks[task.Name]; !ok {
		return errUnknownTask
	}

	q.archived[task.Name] = q.tasks[task.Name]
	delete(q.tasks, task.Name)

	// TODO(vadimsh): If a PULL task, remove from ETA-sorted array.

	return nil
}

func (q *sortedQueue) purge() {
	q.tasks = map[string]*tq.Task{}
	q.archived = map[string]*tq.Task{}
}

func (q *sortedQueue) getStats() *tq.Statistics {
	s := tq.Statistics{
		Tasks: len(q.tasks),
	}
	for _, t := range q.tasks {
		if s.OldestETA.IsZero() {
			s.OldestETA = t.ETA
		} else if t.ETA.Before(s.OldestETA) {
			s.OldestETA = t.ETA
		}
	}
	return &s
}

//////////////////////////////// taskQueueData /////////////////////////////////

type taskQueueData struct {
	sync.Mutex

	queues map[string]*sortedQueue
}

var _ interface {
	memContextObj
	tq.Testable
} = (*taskQueueData)(nil)

func newTaskQueueData() memContextObj {
	return &taskQueueData{
		queues: map[string]*sortedQueue{"default": newSortedQueue("default")},
	}
}

func (t *taskQueueData) canApplyTxn(obj memContextObj) bool { return true }
func (t *taskQueueData) endTxn()                            {}
func (t *taskQueueData) applyTxn(c context.Context, obj memContextObj) {
	txn := obj.(*txnTaskQueueData)
	for qn, tasks := range txn.anony {
		q := t.queues[qn]
		for _, tsk := range tasks {
			// Regenerate names to make sure we don't collide with anything already
			// committed. Note that transactional tasks can't have user-defined name.
			tsk.Name = q.genTaskName(c)
			err := q.addTask(tsk) // prepped in txnTaskQueueData.AddMulti, must be good
			if err != nil {
				panic(err)
			}
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

	if _, ok := t.queues[queueName]; ok {
		panic(fmt.Errorf("memory/taskqueue: cannot add the same queue twice! %q", queueName))
	}
	t.queues[queueName] = newSortedQueue(queueName)
}

func (t *taskQueueData) GetScheduledTasks() tq.QueueData {
	t.Lock()
	defer t.Unlock()

	r := make(tq.QueueData, len(t.queues))
	for qn, q := range t.queues {
		r[qn] = make(map[string]*tq.Task, len(q.tasks))
		for tn, t := range q.tasks {
			r[qn][tn] = t.Duplicate()
		}
	}
	return r
}

func (t *taskQueueData) GetTombstonedTasks() tq.QueueData {
	t.Lock()
	defer t.Unlock()

	r := make(tq.QueueData, len(t.queues))
	for qn, q := range t.queues {
		r[qn] = make(map[string]*tq.Task, len(q.archived))
		for tn, t := range q.archived {
			r[qn][tn] = t.Duplicate()
		}
	}
	return r
}

func (t *taskQueueData) resetTasksWithLock() {
	for _, q := range t.queues {
		q.purge()
	}
}

func (t *taskQueueData) ResetTasks() {
	t.Lock()
	defer t.Unlock()

	t.resetTasksWithLock()
}

func (t *taskQueueData) getQueueLocked(queueName string) (*sortedQueue, error) {
	if queueName == "" {
		queueName = "default"
	}
	q, ok := t.queues[queueName]
	if !ok {
		return nil, errUnknownQueue
	}
	return q, nil
}

func (t *taskQueueData) purgeLocked(queueName string) error {
	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}
	q.purge()
	return nil
}

/////////////////////////////// txnTaskQueueData ///////////////////////////////

type txnTaskQueueData struct {
	lock sync.Mutex

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	anony  tq.AnonymousQueueData
	parent *taskQueueData
}

var _ interface {
	memContextObj
	tq.Testable
} = (*txnTaskQueueData)(nil)

func (t *txnTaskQueueData) canApplyTxn(obj memContextObj) bool { return false }
func (t *txnTaskQueueData) applyTxn(context.Context, memContextObj) {
	impossible(fmt.Errorf("cannot apply nested transaction"))
}
func (t *txnTaskQueueData) mkTxn(*ds.TransactionOptions) memContextObj {
	impossible(fmt.Errorf("cannot start nested transaction"))
	return nil
}

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
