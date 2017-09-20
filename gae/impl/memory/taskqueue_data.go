// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"container/heap"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	prodConstraints "go.chromium.org/gae/impl/prod/constraints"
	ds "go.chromium.org/gae/service/datastore"
	tq "go.chromium.org/gae/service/taskqueue"
)

var (
	currentNamespace = http.CanonicalHeaderKey("X-AppEngine-Current-Namespace")
	defaultNamespace = http.CanonicalHeaderKey("X-AppEngine-Default-Namespace")

	validTaskName = regexp.MustCompile("^[0-9a-zA-Z\\-\\_]{0,500}$")

	errBadRequest       = errors.New("BAD_REQUEST")
	errInvalidTaskName  = errors.New("INVALID_TASK_NAME")
	errUnknownQueue     = errors.New("UNKNOWN_QUEUE")
	errTombstonedTask   = errors.New("TOMBSTONED_TASK")
	errUnknownTask      = errors.New("UNKNOWN_TASK")
	errInvalidQueueMode = errors.New("INVALID_QUEUE_MODE")
	errTaskLeaseExpired = errors.New("TASK_LEASE_EXPIRED")
)

//////////////////////////////// sortedQueue ///////////////////////////////////

type sortedQueue struct {
	name          string
	isPullQueue   bool
	nextAutoGenID uint64

	tasks    map[string]*tq.Task // added, but not deleted
	archived map[string]*tq.Task // tombstones

	sorted       taskIndex             // sorted by (ETA, name)
	sortedPerTag map[string]*taskIndex // tag => tasks sorted by (ETA, name)
}

func newSortedQueue(name string, isPullQueue bool) *sortedQueue {
	// Pick the initial value of the counter based on queue name, so it looks
	// scary. To make sure users don't attempt to "guess" IDs or correlate them
	// between different queues.
	h := fnv.New64()
	h.Write([]byte(name))
	return &sortedQueue{
		name:          name,
		isPullQueue:   isPullQueue,
		nextAutoGenID: h.Sum64(),
		tasks:         map[string]*tq.Task{},
		archived:      map[string]*tq.Task{},
		sortedPerTag:  map[string]*taskIndex{},
	}
}

// All sortedQueue methods are assumed to be called under taskQueueData lock.

func (q *sortedQueue) genTaskName() string {
	q.nextAutoGenID++
	return fmt.Sprintf("%d", q.nextAutoGenID)
}

func (q *sortedQueue) addTask(task *tq.Task) error {
	switch {
	case task.Method == "PULL" && !q.isPullQueue:
		return errInvalidQueueMode
	case task.Method != "PULL" && q.isPullQueue:
		return errInvalidQueueMode
	}

	if _, ok := q.archived[task.Name]; ok {
		// SDK converts TOMBSTONE -> already added too
		return tq.ErrTaskAlreadyAdded
	} else if _, ok := q.tasks[task.Name]; ok {
		return tq.ErrTaskAlreadyAdded
	}

	q.tasks[task.Name] = task

	if q.isPullQueue {
		q.sorted.add(task)

		perTag, ok := q.sortedPerTag[task.Tag]
		if !ok {
			perTag = &taskIndex{}
			q.sortedPerTag[task.Tag] = perTag
		}
		perTag.add(task)
	}

	return nil
}

func (q *sortedQueue) deleteTask(task *tq.Task) error {
	if _, ok := q.archived[task.Name]; ok {
		return errTombstonedTask
	}
	if _, ok := q.tasks[task.Name]; !ok {
		return errUnknownTask
	}

	t := q.tasks[task.Name]
	q.archived[task.Name] = t
	delete(q.tasks, task.Name)

	if q.isPullQueue {
		q.sorted.remove(t)
		q.sortedPerTag[t.Tag].remove(t)
	}

	return nil
}

func (q *sortedQueue) leaseTasks(now time.Time, maxTasks int, leaseTime time.Duration, useTag bool, tag string) ([]*tq.Task, error) {
	if !q.isPullQueue {
		return nil, errInvalidQueueMode
	}

	if maxTasks <= 0 {
		return nil, errBadRequest
	}
	leaseSec := int(leaseTime / time.Second)
	if leaseSec < 0 {
		return nil, errBadRequest
	}

	if !useTag && tag != "" {
		panic("taskqueue: impossible leaseTasks call")
	}

	// useTag == true and tag == "" is VALID request here. It means "find
	// the first task by ETA, and use its tag as if it was passed to leaseTasks,
	// or fetch only untagged tasks if the tag is not set". That's how production
	// API works too.
	if useTag && tag == "" {
		// Fetch the first task with ETA <= now to examine its tag.
		task := q.sorted.peek()
		if task == nil || task.ETA.After(now) {
			return nil, nil // no ready tasks at all
		}
		// It is possible 'tag' is "" here. It means "fetch only untagged tasks".
		tag = task.Tag
	}

	// Extract all the tasks that match the criteria. We'll update their ETA and
	// push them back into the index (at updated positions).
	var tasks []*tq.Task
	if useTag {
		if perTag := q.sortedPerTag[tag]; perTag != nil {
			tasks = perTag.extract(now, maxTasks)
			for _, t := range tasks {
				q.sorted.remove(t)
			}
		}
	} else {
		tasks = q.sorted.extract(now, maxTasks)
		for _, t := range tasks {
			q.sortedPerTag[t.Tag].remove(t)
		}
	}

	// Seconds precision is important, that's how production API works.
	newETA := now.Add(time.Duration(leaseSec) * time.Second)
	for _, t := range tasks {
		t.ETA = newETA
		q.sorted.add(t)
		q.sortedPerTag[t.Tag].add(t)
	}

	out := make([]*tq.Task, len(tasks))
	for i := range tasks {
		out[i] = tasks[i].Duplicate()
	}
	return out, nil
}

func (q *sortedQueue) modifyTaskLease(now time.Time, t *tq.Task, leaseTime time.Duration) error {
	if !q.isPullQueue {
		return errInvalidQueueMode
	}

	leaseSec := int(leaseTime / time.Second)
	if leaseSec < 0 {
		return errBadRequest
	}

	if _, ok := q.archived[t.Name]; ok {
		return errTombstonedTask
	}
	if _, ok := q.tasks[t.Name]; !ok {
		return errUnknownTask
	}

	// Check ownership of the task by using ETA field as a "cookie". Clients are
	// supposed to round-trip the ETA they get from 'leaseTasks' back to
	// 'modifyLease'. Production API works the same way.
	task := q.tasks[t.Name]
	if !task.ETA.Equal(t.ETA) {
		return errTaskLeaseExpired
	}

	// The lease has been lost by timeout.
	if now.After(task.ETA) {
		return errTaskLeaseExpired
	}

	// Update the lease and the indexes. Seconds precision is important, that's
	// how production API works.
	q.sorted.remove(task)
	q.sortedPerTag[task.Tag].remove(task)
	task.ETA = now.Add(time.Duration(leaseSec) * time.Second)
	q.sorted.add(task)
	q.sortedPerTag[task.Tag].add(task)

	// Make the caller know the new ETA, in case it needs to be passed to
	// 'modifyLease' again.
	t.ETA = task.ETA
	return nil
}

func (q *sortedQueue) purge() {
	q.tasks = map[string]*tq.Task{}
	q.archived = map[string]*tq.Task{}
	q.sorted = taskIndex{}
	q.sortedPerTag = map[string]*taskIndex{}
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

/////////////////////////////// Indexing helpers ///////////////////////////////

// taskIndex is a heap of tasks sorted by (ETA, name), oldest first.
type taskIndex struct {
	data taskIndexData
}

// add puts the task into the index, complexity is O(log(N)).
func (idx *taskIndex) add(t *tq.Task) {
	heap.Push(&idx.data, t)
}

// remove deletes the task from the index, complexity is O(N).
func (idx *taskIndex) remove(t *tq.Task) {
	for i, task := range idx.data {
		if t == task {
			heap.Remove(&idx.data, i)
			return
		}
	}
}

// peek returns the task with the minimum ETA, complexity is O(1).
func (idx *taskIndex) peek() *tq.Task {
	if len(idx.data) == 0 {
		return nil
	}
	return idx.data[0]
}

// extract finds up to 'max' tasks with ETA <= now and removes them.
//
// Returns them as well, preserving the order (the first returned task is the
// oldest).
//
// Complexity is O(log(N)*max).
func (idx *taskIndex) extract(now time.Time, max int) []*tq.Task {
	var out []*tq.Task
	for len(idx.data) > 0 && len(out) < max && !idx.data[0].ETA.After(now) {
		out = append(out, heap.Pop(&idx.data).(*tq.Task))
	}
	return out
}

// taskIndexData implements heap.Interface.
type taskIndexData []*tq.Task

func (d taskIndexData) Len() int      { return len(d) }
func (d taskIndexData) Swap(i, j int) { d[i], d[j] = d[j], d[i] }

func (d taskIndexData) Less(i, j int) bool {
	if d[i].ETA.Equal(d[j].ETA) {
		return d[i].Name < d[j].Name
	}
	return d[i].ETA.Before(d[j].ETA)
}

func (d *taskIndexData) Push(x interface{}) {
	*d = append(*d, x.(*tq.Task))
}

func (d *taskIndexData) Pop() interface{} {
	old := *d
	n := len(old)
	x := old[n-1]
	*d = old[0 : n-1]
	return x
}

//////////////////////////////// taskQueueData /////////////////////////////////

type taskQueueData struct {
	lock        sync.Mutex
	queues      map[string]*sortedQueue
	constraints tq.Constraints
}

var _ memContextObj = (*taskQueueData)(nil)

func newTaskQueueData() memContextObj {
	return &taskQueueData{
		queues:      map[string]*sortedQueue{"default": newSortedQueue("default", false)},
		constraints: prodConstraints.TQ(),
	}
}

func (t *taskQueueData) endTxn() {}

func (t *taskQueueData) beginCommit(c context.Context, txnCtxObj memContextObj) txnCommitOp {
	txn := txnCtxObj.(*txnTaskQueueData)

	txn.lock.Lock() // no need to hold t.lock, since no collisions are possible

	return &txnCommitCallback{
		unlock: txn.lock.Unlock,
		apply: func() {
			t.lock.Lock()
			defer t.lock.Unlock()
			for qn, tasks := range txn.anony {
				q := t.queues[qn]
				for _, tsk := range tasks {
					err := q.addTask(tsk) // prepped in txnTaskQueueData.AddMulti, must be good
					if err != nil {
						panic(err)
					}
				}
			}
			txn.anony = nil
		},
	}
}

func (t *taskQueueData) mkTxn(*ds.TransactionOptions) memContextObj {
	return &txnTaskQueueData{
		parent: t,
		anony:  tq.AnonymousQueueData{},
	}
}

func (t *taskQueueData) getTransactionTasks(ns string) tq.AnonymousQueueData { return nil }

func (t *taskQueueData) createQueue(queueName string) {
	t.createQueueInternal(queueName, false)
}

func (t *taskQueueData) createPullQueue(queueName string) {
	t.createQueueInternal(queueName, true)
}

func (t *taskQueueData) createQueueInternal(queueName string, isPullQueue bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if _, ok := t.queues[queueName]; ok {
		panic(fmt.Errorf("memory/taskqueue: cannot add the same queue twice! %q", queueName))
	}
	t.queues[queueName] = newSortedQueue(queueName, isPullQueue)
}

func (t *taskQueueData) getScheduledTasks(ns string) tq.QueueData {
	t.lock.Lock()
	defer t.lock.Unlock()

	r := make(tq.QueueData, len(t.queues))
	for qn, q := range t.queues {
		r[qn] = make(map[string]*tq.Task, len(q.tasks))
		for tn, t := range q.tasks {
			if taskNamespace(t) == ns {
				r[qn][tn] = t.Duplicate()
			}
		}
	}
	return r
}

func (t *taskQueueData) getTombstonedTasks(ns string) tq.QueueData {
	t.lock.Lock()
	defer t.lock.Unlock()

	r := make(tq.QueueData, len(t.queues))
	for qn, q := range t.queues {
		r[qn] = make(map[string]*tq.Task, len(q.archived))
		for tn, t := range q.archived {
			if taskNamespace(t) == ns {
				r[qn][tn] = t.Duplicate()
			}
		}
	}
	return r
}

func (t *taskQueueData) resetTasks() {
	t.lock.Lock()
	defer t.lock.Unlock()

	for _, q := range t.queues {
		q.purge()
	}
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

func (t *taskQueueData) setConstraints(c *tq.Constraints) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if c == nil {
		t.constraints = tq.Constraints{}
	} else {
		t.constraints = *c
	}
}

func (t *taskQueueData) getConstraints() tq.Constraints {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.constraints
}

/////////////////////////////// txnTaskQueueData ///////////////////////////////

type txnTaskQueueData struct {
	lock sync.Mutex

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	anony  tq.AnonymousQueueData
	parent *taskQueueData
}

var _ memContextObj = (*txnTaskQueueData)(nil)

func (t *txnTaskQueueData) mkTxn(*ds.TransactionOptions) memContextObj {
	impossible(fmt.Errorf("cannot start nested transaction"))
	return nil
}

func (*txnTaskQueueData) beginCommit(c context.Context, txnCtxObj memContextObj) txnCommitOp {
	impossible(fmt.Errorf("cannot commit a nested transaction"))
	return nil
}

func (t *txnTaskQueueData) endTxn() {
	if atomic.LoadInt32(&t.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&t.closed, 1)
}

func (t *txnTaskQueueData) resetTasks() {
	t.lock.Lock()
	for queuename := range t.anony {
		t.anony[queuename] = nil
	}
	t.lock.Unlock()

	t.parent.resetTasks()
}

func (t *txnTaskQueueData) getTransactionTasks(ns string) tq.AnonymousQueueData {
	t.lock.Lock()
	defer t.lock.Unlock()

	ret := make(tq.AnonymousQueueData, len(t.anony))
	for k, vs := range t.anony {
		ret[k] = make([]*tq.Task, len(vs))
		for i, v := range vs {
			if taskNamespace(v) == ns {
				ret[k][i] = v.Duplicate()
			}
		}
	}

	return ret
}

func (t *txnTaskQueueData) getTombstonedTasks(ns string) tq.QueueData {
	return t.parent.getTombstonedTasks(ns)
}

func (t *txnTaskQueueData) getScheduledTasks(ns string) tq.QueueData {
	return t.parent.getScheduledTasks(ns)
}

func (t *txnTaskQueueData) createQueue(queueName string) {
	t.parent.createQueue(queueName)
}

func (t *txnTaskQueueData) createPullQueue(queueName string) {
	t.parent.createPullQueue(queueName)
}

// taskQueueTestable is a tq.Testable implementation that is bound to a
// specified namespace.
type taskQueueTestable struct {
	ns   string
	data interface {
		resetTasks()
		getTombstonedTasks(ns string) tq.QueueData
		getScheduledTasks(ns string) tq.QueueData
		getTransactionTasks(ns string) tq.AnonymousQueueData
		createQueue(queueName string)
		createPullQueue(queueName string)
	}
}

func (t *taskQueueTestable) ResetTasks() { t.data.resetTasks() }
func (t *taskQueueTestable) GetTombstonedTasks() tq.QueueData {
	return t.data.getTombstonedTasks(t.ns)
}
func (t *taskQueueTestable) GetScheduledTasks() tq.QueueData {
	return t.data.getScheduledTasks(t.ns)
}
func (t *taskQueueTestable) GetTransactionTasks() tq.AnonymousQueueData {
	return t.data.getTransactionTasks(t.ns)
}
func (t *taskQueueTestable) CreateQueue(queueName string)     { t.data.createQueue(queueName) }
func (t *taskQueueTestable) CreatePullQueue(queueName string) { t.data.createPullQueue(queueName) }
