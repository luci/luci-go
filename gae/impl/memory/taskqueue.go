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
	// Reject the entire batch if at least one task is bad. That's how prod API
	// behaves too.
	if err := checkManyTasks(tasks, false); err != nil {
		return err
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		name := task.Name
		if name == "" {
			name = q.genTaskName()
		}
		task = prepTask(t.ctx, task, name, q.name, t.ns)
		err := q.addTask(task)
		if err != nil {
			cb(nil, err)
		} else {
			cb(task.Duplicate(), nil)
		}
	}
	return nil
}

func (t *taskqueueImpl) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	t.lock.Lock()
	defer t.lock.Unlock()

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
	t.lock.Lock()
	defer t.lock.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return nil, err
	}

	return q.leaseTasks(clock.Now(t.ctx), maxTasks, leaseTime, false, "")
}

func (t *taskqueueImpl) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*tq.Task, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return nil, err
	}

	return q.leaseTasks(clock.Now(t.ctx), maxTasks, leaseTime, true, tag)
}

func (t *taskqueueImpl) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	q, err := t.getQueueLocked(queueName)
	if err != nil {
		return err
	}

	return q.modifyTaskLease(clock.Now(t.ctx), task, leaseTime)
}

func (t *taskqueueImpl) Purge(queueName string) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.purgeLocked(queueName)
}

func (t *taskqueueImpl) Stats(queueNames []string, cb tq.RawStatsCB) error {
	t.lock.Lock()
	defer t.lock.Unlock()

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

func (t *taskqueueImpl) SetConstraints(c *tq.Constraints) error {
	t.setConstraints(c)
	return nil
}

func (t *taskqueueImpl) Constraints() tq.Constraints {
	return t.getConstraints()
}

func (t *taskqueueImpl) GetTestable() tq.Testable { return &taskQueueTestable{t.ns, t} }

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	*txnTaskQueueData

	ctx context.Context
	ns  string
}

var _ tq.RawInterface = (*taskqueueTxnImpl)(nil)

func (t *taskqueueTxnImpl) addLocked(task *tq.Task, taskName, queueName string) (*tq.Task, error) {
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

	toSched := prepTask(t.ctx, task, taskName, queueName, t.ns)
	t.anony[queueName] = append(t.anony[queueName], toSched)
	return toSched.Duplicate(), nil
}

func (t *taskqueueTxnImpl) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	if err := assertTxnValid(t.ctx); err != nil {
		return err
	}

	// Reject the entire batch if at least one task is bad. That's how prod API
	// behaves too.
	if err := checkManyTasks(tasks, true); err != nil {
		return err
	}

	// Generate names for all tasks.
	names := make([]string, len(tasks))
	t.parent.lock.Lock()
	q, err := t.parent.getQueueLocked(queueName)
	if err == nil {
		queueName = q.name
		for i := range tasks {
			names[i] = q.genTaskName()
		}
	}
	t.parent.lock.Unlock()
	if err != nil {
		return err
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	for i, task := range tasks {
		cb(t.addLocked(task, names[i], queueName))
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

func (t *taskqueueTxnImpl) GetTestable() tq.Testable { return &taskQueueTestable{t.ns, t} }

////////////////////////// private functions ///////////////////////////////////

// checkTask ensures the task properties (in particular name and method, as
// passed by the user) are acceptable.
//
// Only empty name is allowed in transactions (the name will be auto generated
// later).
func checkTask(task *tq.Task, isTxn bool) error {
	switch {
	case task == nil:
		return fmt.Errorf("taskqueue: the task can't be nil")
	case isTxn && task.Name != "":
		return fmt.Errorf("taskqueue: INVALID_TASK_NAME: cannot add named task %q in transaction", task.Name)
	case task.Name != "" && !validTaskName.MatchString(task.Name):
		return errInvalidTaskName
	}

	switch task.Method {
	case "", "POST", "PUT", "PULL", "GET", "HEAD", "DELETE":
		// good methods
	default:
		return fmt.Errorf("taskqueue: bad method %q", task.Method)
	}

	return nil
}

// checkManyTasks is a batch variant of checkTask that returns a multi error.
func checkManyTasks(tasks []*tq.Task, isTxn bool) error {
	lme := errors.NewLazyMultiError(len(tasks))
	for i, t := range tasks {
		lme.Assign(i, checkTask(t, isTxn))
	}
	return lme.Get()
}

// prepTask clones 'task' and fills in its properties (including name).
//
// We need to clone the task, since per Task Queues API AddMulti method returns
// a modified copy of the task, without actually touching the original.
//
// Assumes the passed is already validated via checkTask. It overrides whatever
// name is specified in task.Name with taskName.
func prepTask(c context.Context, task *tq.Task, taskName, queueName, ns string) *tq.Task {
	if taskName == "" {
		panic("taskqueue: taskName should be auto-generated already, if necessary")
	}

	toSched := task.Duplicate()
	toSched.Name = taskName

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
		panic("taskqueue: task.Method should have been validated already")
	}

	// PULL tasks have no HTTP related stuff in them (Path and Header).
	if toSched.Method == "PULL" {
		toSched.Path = ""
		toSched.Header = nil
	} else {
		if toSched.Path == "" {
			toSched.Path = "/_ah/queue/" + queueName
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

	return toSched
}

func taskNamespace(task *tq.Task) string { return task.Header.Get(currentNamespace) }
