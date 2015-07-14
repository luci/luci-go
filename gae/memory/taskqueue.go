// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"infra/gae/libs/wrapper"
	"net/http"
	"regexp"

	"golang.org/x/net/context"

	"appengine"
	"appengine/taskqueue"
	"appengine_internal"
	dbpb "appengine_internal/datastore"
	pb "appengine_internal/taskqueue"
)

/////////////////////////////// public functions ///////////////////////////////

func useTQ(c context.Context) context.Context {
	return wrapper.SetTQFactory(c, func(ic context.Context) wrapper.TaskQueue {
		tqd := cur(ic).Get(memContextTQIdx)
		var ret interface {
			wrapper.TQTestable
			wrapper.TaskQueue
		}
		switch x := tqd.(type) {
		case *taskQueueData:
			ret = &taskqueueImpl{
				wrapper.DummyTQ(),
				x,
				ic,
				curGID(ic).namespace,
			}

		case *txnTaskQueueData:
			ret = &taskqueueTxnImpl{
				wrapper.DummyTQ(),
				x,
				ic,
				curGID(ic).namespace,
			}

		default:
			panic(fmt.Errorf("TQ: bad type: %v", tqd))
		}
		return ret
	})
}

//////////////////////////////// taskqueueImpl /////////////////////////////////

type taskqueueImpl struct {
	wrapper.TaskQueue
	*taskQueueData

	ctx context.Context
	ns  string
}

var (
	_ = wrapper.TaskQueue((*taskqueueImpl)(nil))
	_ = wrapper.TQTestable((*taskqueueImpl)(nil))
)

func (t *taskqueueImpl) addLocked(task *taskqueue.Task, queueName string) (*taskqueue.Task, error) {
	toSched, queueName, err := t.prepTask(t.ctx, t.ns, task, queueName)
	if err != nil {
		return nil, err
	}

	if _, ok := t.archived[queueName][toSched.Name]; ok {
		// SDK converts TOMBSTONE -> already added too
		return nil, taskqueue.ErrTaskAlreadyAdded
	} else if _, ok := t.named[queueName][toSched.Name]; ok {
		return nil, taskqueue.ErrTaskAlreadyAdded
	} else {
		t.named[queueName][toSched.Name] = toSched
	}

	return dupTask(toSched), nil
}

func (t *taskqueueImpl) Add(task *taskqueue.Task, queueName string) (*taskqueue.Task, error) {
	if err := t.IsBroken(); err != nil {
		return nil, err
	}

	t.Lock()
	defer t.Unlock()

	return t.addLocked(task, queueName)
}

func (t *taskqueueImpl) deleteLocked(task *taskqueue.Task, queueName string) error {
	queueName, err := t.getQueueName(queueName)
	if err != nil {
		return err
	}

	if _, ok := t.archived[queueName][task.Name]; ok {
		return newTQError(pb.TaskQueueServiceError_TOMBSTONED_TASK)
	}

	if _, ok := t.named[queueName][task.Name]; !ok {
		return newTQError(pb.TaskQueueServiceError_UNKNOWN_TASK)
	}

	t.archived[queueName][task.Name] = t.named[queueName][task.Name]
	delete(t.named[queueName], task.Name)

	return nil
}

func (t *taskqueueImpl) Delete(task *taskqueue.Task, queueName string) error {
	if err := t.IsBroken(); err != nil {
		return err
	}

	t.Lock()
	defer t.Unlock()

	return t.deleteLocked(task, queueName)
}

func (t *taskqueueImpl) AddMulti(tasks []*taskqueue.Task, queueName string) ([]*taskqueue.Task, error) {
	if err := t.IsBroken(); err != nil {
		return nil, err
	}

	t.Lock()
	defer t.Unlock()

	return multi(tasks, queueName, t.addLocked)
}

func (t *taskqueueImpl) DeleteMulti(tasks []*taskqueue.Task, queueName string) error {
	if err := t.IsBroken(); err != nil {
		return err
	}

	t.Lock()
	defer t.Unlock()

	_, err := multi(tasks, queueName,
		func(tsk *taskqueue.Task, qn string) (*taskqueue.Task, error) {
			return nil, t.deleteLocked(tsk, qn)
		})
	return err
}

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	wrapper.TaskQueue
	*txnTaskQueueData

	ctx context.Context
	ns  string
}

var (
	_ = wrapper.TaskQueue((*taskqueueTxnImpl)(nil))
	_ = wrapper.TQTestable((*taskqueueTxnImpl)(nil))
)

func (t *taskqueueTxnImpl) addLocked(task *taskqueue.Task, queueName string) (*taskqueue.Task, error) {
	toSched, queueName, err := t.parent.prepTask(t.ctx, t.ns, task, queueName)
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
		return nil, newDSError(dbpb.Error_BAD_REQUEST)
	}

	t.anony[queueName] = append(t.anony[queueName], toSched)

	// the fact that we have generated a unique name for this task queue item is
	// an implementation detail.
	// TODO(riannucci): now that I think about this... it may not actually be true.
	//		We should verify that the .Name for a task added in a transaction is
	//		meaningless. Maybe names generated in a transaction are somehow
	//		guaranteed to be meaningful?
	toRet := dupTask(toSched)
	toRet.Name = ""

	return toRet, nil
}

func (t *taskqueueTxnImpl) Add(task *taskqueue.Task, queueName string) (*taskqueue.Task, error) {
	if err := t.IsBroken(); err != nil {
		return nil, err
	}

	t.Lock()
	defer t.Unlock()

	return t.addLocked(task, queueName)
}

func (t *taskqueueTxnImpl) AddMulti(tasks []*taskqueue.Task, queueName string) ([]*taskqueue.Task, error) {
	if err := t.IsBroken(); err != nil {
		return nil, err
	}

	t.Lock()
	defer t.Unlock()

	return multi(tasks, queueName, t.addLocked)
}

////////////////////////////// private functions ///////////////////////////////

var validTaskName = regexp.MustCompile("^[0-9a-zA-Z\\-\\_]{0,500}$")

const validTaskChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"

func mkName(c context.Context, cur string, queue map[string]*taskqueue.Task) string {
	_, ok := queue[cur]
	for !ok && cur == "" {
		name := [500]byte{}
		for i := 0; i < 500; i++ {
			name[i] = validTaskChars[wrapper.GetMathRand(c).Intn(len(validTaskChars))]
		}
		cur = string(name[:])
		_, ok = queue[cur]
	}
	return cur
}

func newTQError(code pb.TaskQueueServiceError_ErrorCode) *appengine_internal.APIError {
	return &appengine_internal.APIError{Service: "taskqueue", Code: int32(code)}
}

func multi(tasks []*taskqueue.Task, queueName string, f func(*taskqueue.Task, string) (*taskqueue.Task, error)) ([]*taskqueue.Task, error) {
	ret := []*taskqueue.Task(nil)
	me := appengine.MultiError(nil)
	foundErr := false
	for _, task := range tasks {
		rt, err := f(task, queueName)
		ret = append(ret, rt)
		me = append(me, err)
		if err != nil {
			foundErr = true
		}
	}
	if !foundErr {
		me = nil
	}
	return ret, me
}

func dupTask(t *taskqueue.Task) *taskqueue.Task {
	ret := &taskqueue.Task{}
	*ret = *t

	if t.Header != nil {
		ret.Header = make(http.Header, len(t.Header))
		for k, vs := range t.Header {
			newVs := make([]string, len(vs))
			copy(newVs, vs)
			ret.Header[k] = newVs
		}
	}

	if t.Payload != nil {
		ret.Payload = make([]byte, len(t.Payload))
		copy(ret.Payload, t.Payload)
	}

	if t.RetryOptions != nil {
		ret.RetryOptions = &taskqueue.RetryOptions{}
		*ret.RetryOptions = *t.RetryOptions
	}

	return ret
}

func dupQueue(q wrapper.QueueData) wrapper.QueueData {
	r := make(wrapper.QueueData, len(q))
	for k, q := range q {
		r[k] = make(map[string]*taskqueue.Task, len(q))
		for tn, t := range q {
			r[k][tn] = dupTask(t)
		}
	}
	return r
}
