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
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	tq "go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

/////////////////////////////// public functions ///////////////////////////////

func useTQ(c context.Context) context.Context {
	return tq.SetRawFactory(c, func(ic context.Context) tq.RawInterface {
		memCtx, isTxn := cur(ic)
		tqd := memCtx.Get(memContextTQIdx)

		ns := info.GetNamespace(ic)
		if isTxn {
			return &taskqueueTxnImpl{tqd.(*txnTaskQueueData), ic, ns}
		}
		return &taskqueueImpl{tqd.(*taskQueueData), ic, ns}
	})
}

//////////////////////////////// taskqueueImpl /////////////////////////////////

type taskqueueImpl struct {
	*taskQueueData

	ctx context.Context
	ns  string
}

var _ tq.RawInterface = (*taskqueueImpl)(nil)

func (t *taskqueueImpl) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	t.Lock()
	defer t.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		task, err := prepTask(t.ctx, task, q, t.ns, false)
		if err == nil {
			err = q.addTask(task)
		}
		if err != nil {
			cb(nil, err)
		} else {
			cb(task.Duplicate(), nil)
		}
	}
	return nil
}

func (t *taskqueueImpl) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	t.Lock()
	defer t.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	for i, task := range tasks {
		if err := q.deleteTask(task); err != nil {
			cb(i, err)
		}
	}
	return nil
}

func (t *taskqueueImpl) Lease(maxTasks int, queueName string, leaseTime time.Duration) ([]*tq.Task, error) {
	t.Lock()
	defer t.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return nil, err
	}

	return q.leaseTasks(clock.Now(t.ctx), maxTasks, leaseTime, false, "")
}

func (t *taskqueueImpl) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*tq.Task, error) {
	t.Lock()
	defer t.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return nil, err
	}

	return q.leaseTasks(clock.Now(t.ctx), maxTasks, leaseTime, true, tag)
}

func (t *taskqueueImpl) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	t.Lock()
	defer t.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	return q.modifyTaskLease(clock.Now(t.ctx), task, leaseTime)
}

func (t *taskqueueImpl) Purge(queueName string) error {
	t.Lock()
	defer t.Unlock()

	return t.purgeLocked(queueName)
}

func (t *taskqueueImpl) Stats(queueNames []string, cb tq.RawStatsCB) error {
	t.Lock()
	defer t.Unlock()

	for _, qn := range queueNames {
		q, err := t.getQueueLocked(qn)
		if err != nil {
			cb(nil, err)
		} else {
			cb(q.getStats(), nil)
		}
	}

	return nil
}

func (t *taskqueueImpl) Constraints() tq.Constraints {
	return t.taskQueueData.getConstraints()
}

func (t *taskqueueImpl) GetTestable() tq.Testable { return &taskQueueTestable{t.ns, t} }

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	*txnTaskQueueData

	ctx context.Context
	ns  string
}

var _ tq.RawInterface = (*taskqueueTxnImpl)(nil)

func (t *taskqueueTxnImpl) addLocked(task *tq.Task, q *sortedQueue) (*tq.Task, error) {
	toSched, err := prepTask(t.ctx, task, q, t.ns, true)
	if err != nil {
		return nil, err
	}

	numTasks := 0
	for _, vs := range t.anony {
		numTasks += len(vs)
	}
	if numTasks+1 > 5 {
		// transactional tasks are actually implemented 'for real' as Actions which
		// ride on the datastore. The current datastore implementation only allows
		// a maximum of 5 Actions per transaction, and more than that result in a
		// BAD_REQUEST.
		return nil, errBadRequest
	}

	t.anony[q.name] = append(t.anony[q.name], toSched)

	// the fact that we have generated a unique name for this task queue item is
	// an implementation detail. These names are regenerated when the transaction
	// is committed.
	// TODO(riannucci): now that I think about this... it may not actually be true.
	//		We should verify that the .Name for a task added in a transaction is
	//		meaningless. Maybe names generated in a transaction are somehow
	//		guaranteed to be meaningful?
	toRet := toSched.Duplicate()
	toRet.Name = ""

	return toRet, nil
}

func (t *taskqueueTxnImpl) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	if err := assertTxnValid(t.ctx); err != nil {
		return err
	}

	t.Lock()
	defer t.Unlock()

	q, err := t.parent.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		cb(t.addLocked(task, q))
	}
	return nil
}

func (t *taskqueueTxnImpl) DeleteMulti([]*tq.Task, string, tq.RawCB) error {
	return errors.New("taskqueue: cannot DeleteMulti from a transaction")
}

func (t *taskqueueTxnImpl) Lease(maxTasks int, queueName string, leaseTime time.Duration) ([]*tq.Task, error) {
	return nil, errors.New("taskqueue: cannot Lease from a transaction")
}

func (t *taskqueueTxnImpl) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*tq.Task, error) {
	return nil, errors.New("taskqueue: cannot LeaseByTag from a transaction")
}

func (t *taskqueueTxnImpl) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	return errors.New("taskqueue: cannot ModifyLease from a transaction")
}

func (t *taskqueueTxnImpl) Constraints() tq.Constraints {
	return t.parent.getConstraints()
}

func (t *taskqueueTxnImpl) Purge(string) error {
	return errors.New("taskqueue: cannot Purge from a transaction")
}

func (t *taskqueueTxnImpl) Stats([]string, tq.RawStatsCB) error {
	return errors.New("taskqueue: cannot Stats from a transaction")
}

func (t *taskqueueImpl) SetConstraints(c *tq.Constraints) error {
	if c == nil {
		c = &tq.Constraints{}
	}

	t.Lock()
	defer t.Unlock()
	t.setConstraintsLocked(*c)
	return nil
}

func (t *taskqueueTxnImpl) GetTestable() tq.Testable { return &taskQueueTestable{t.ns, t} }

////////////////////////// private functions ///////////////////////////////////

func prepTask(c context.Context, task *tq.Task, q *sortedQueue, ns string, inTxn bool) (*tq.Task, error) {
	toSched := task.Duplicate()

	if inTxn && task.Name != "" {
		return nil, fmt.Errorf("taskqueue: INVALID_TASK_NAME: cannot add named task %q in transaction", task.Name)
	}

	if toSched.ETA.IsZero() {
		toSched.ETA = clock.Now(c).Add(toSched.Delay)
	} else if toSched.Delay != 0 {
		panic("taskqueue: both Delay and ETA are set")
	}
	toSched.Delay = 0

	switch toSched.Method {
	// Methods that can have payloads.
	case "":
		toSched.Method = "POST"
		fallthrough
	case "POST", "PUT", "PULL":
		break

	// Methods that can not have payloads.
	case "GET", "HEAD", "DELETE":
		toSched.Payload = nil

	default:
		return nil, fmt.Errorf("taskqueue: bad method %q", toSched.Method)
	}

	// PULL tasks have no HTTP related stuff in them (Path and Header).
	if toSched.Method == "PULL" {
		toSched.Path = ""
		toSched.Header = nil
	} else {
		if toSched.Path == "" {
			toSched.Path = "/_ah/queue/" + q.name
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
	}

	if toSched.Name == "" {
		toSched.Name = q.genTaskName(c)
	} else {
		if !validTaskName.MatchString(toSched.Name) {
			return nil, errInvalidTaskName
		}
	}

	return toSched, nil
}

func taskNamespace(task *tq.Task) string { return task.Header.Get(currentNamespace) }
