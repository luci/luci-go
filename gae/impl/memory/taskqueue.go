// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"net/http"
	"regexp"

	"golang.org/x/net/context"

	"github.com/luci/gae"
	"github.com/luci/gae/impl/dummy"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/mathrand"
)

/////////////////////////////// public functions ///////////////////////////////

func useTQ(c context.Context) context.Context {
	return tq.SetFactory(c, func(ic context.Context) tq.Interface {
		tqd := cur(ic).Get(memContextTQIdx)
		if x, ok := tqd.(*taskQueueData); ok {
			return &taskqueueImpl{
				dummy.TaskQueue(),
				x,
				ic,
				curGID(ic).namespace,
			}
		}
		return &taskqueueTxnImpl{
			dummy.TaskQueue(),
			tqd.(*txnTaskQueueData),
			ic,
			curGID(ic).namespace,
		}
	})
}

//////////////////////////////// taskqueueImpl /////////////////////////////////

type taskqueueImpl struct {
	tq.Interface
	*taskQueueData

	ctx context.Context
	ns  string
}

var (
	_ = tq.Interface((*taskqueueImpl)(nil))
	_ = tq.Testable((*taskqueueImpl)(nil))
)

func (t *taskqueueImpl) addLocked(task *tq.Task, queueName string) (*tq.Task, error) {
	toSched, queueName, err := t.prepTask(t.ctx, t.ns, task, queueName)
	if err != nil {
		return nil, err
	}

	if _, ok := t.archived[queueName][toSched.Name]; ok {
		// SDK converts TOMBSTONE -> already added too
		return nil, tq.ErrTaskAlreadyAdded
	} else if _, ok := t.named[queueName][toSched.Name]; ok {
		return nil, tq.ErrTaskAlreadyAdded
	} else {
		t.named[queueName][toSched.Name] = toSched
	}

	return dupTask(toSched), nil
}

func (t *taskqueueImpl) Add(task *tq.Task, queueName string) (*tq.Task, error) {
	t.Lock()
	defer t.Unlock()
	return t.addLocked(task, queueName)
}

func (t *taskqueueImpl) deleteLocked(task *tq.Task, queueName string) error {
	queueName, err := t.getQueueName(queueName)
	if err != nil {
		return err
	}

	if _, ok := t.archived[queueName][task.Name]; ok {
		return errors.New("TOMBSTONED_TASK")
	}

	if _, ok := t.named[queueName][task.Name]; !ok {
		return errors.New("UNKNOWN_TASK")
	}

	t.archived[queueName][task.Name] = t.named[queueName][task.Name]
	delete(t.named[queueName], task.Name)

	return nil
}

func (t *taskqueueImpl) Delete(task *tq.Task, queueName string) error {
	t.Lock()
	defer t.Unlock()
	return t.deleteLocked(task, queueName)
}

func (t *taskqueueImpl) AddMulti(tasks []*tq.Task, queueName string) ([]*tq.Task, error) {
	t.Lock()
	defer t.Unlock()
	return multi(tasks, queueName, t.addLocked)
}

func (t *taskqueueImpl) DeleteMulti(tasks []*tq.Task, queueName string) error {
	t.Lock()
	defer t.Unlock()

	_, err := multi(tasks, queueName,
		func(tsk *tq.Task, qn string) (*tq.Task, error) {
			return nil, t.deleteLocked(tsk, qn)
		})
	return err
}

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	tq.Interface
	*txnTaskQueueData

	ctx context.Context
	ns  string
}

var _ interface {
	tq.Interface
	tq.Testable
} = (*taskqueueTxnImpl)(nil)

func (t *taskqueueTxnImpl) addLocked(task *tq.Task, queueName string) (*tq.Task, error) {
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
		return nil, errors.New("BAD_REQUEST")
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

func (t *taskqueueTxnImpl) Add(task *tq.Task, queueName string) (retTask *tq.Task, err error) {
	err = t.run(func() (err error) {
		t.Lock()
		defer t.Unlock()
		retTask, err = t.addLocked(task, queueName)
		return
	})
	return
}

func (t *taskqueueTxnImpl) AddMulti(tasks []*tq.Task, queueName string) (retTasks []*tq.Task, err error) {
	err = t.run(func() (err error) {
		t.Lock()
		defer t.Unlock()
		retTasks, err = multi(tasks, queueName, t.addLocked)
		return
	})
	return
}

////////////////////////////// private functions ///////////////////////////////

var validTaskName = regexp.MustCompile("^[0-9a-zA-Z\\-\\_]{0,500}$")

const validTaskChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"

func mkName(c context.Context, cur string, queue map[string]*tq.Task) string {
	_, ok := queue[cur]
	for !ok && cur == "" {
		name := [500]byte{}
		for i := 0; i < 500; i++ {
			name[i] = validTaskChars[mathrand.Get(c).Intn(len(validTaskChars))]
		}
		cur = string(name[:])
		_, ok = queue[cur]
	}
	return cur
}

func multi(tasks []*tq.Task, queueName string, f func(*tq.Task, string) (*tq.Task, error)) ([]*tq.Task, error) {
	ret := []*tq.Task(nil)
	lme := gae.LazyMultiError{Size: len(tasks)}
	for i, task := range tasks {
		rt, err := f(task, queueName)
		ret = append(ret, rt)
		lme.Assign(i, err)
	}
	return ret, lme.Get()
}

func dupTask(t *tq.Task) *tq.Task {
	ret := &tq.Task{}
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
		ret.RetryOptions = &tq.RetryOptions{}
		*ret.RetryOptions = *t.RetryOptions
	}

	return ret
}

func dupQueue(q tq.QueueData) tq.QueueData {
	r := make(tq.QueueData, len(q))
	for k, q := range q {
		r[k] = make(map[string]*tq.Task, len(q))
		for tn, t := range q {
			r[k][tn] = dupTask(t)
		}
	}
	return r
}
