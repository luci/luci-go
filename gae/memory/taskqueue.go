// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"golang.org/x/net/context"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/dummy"
)

/////////////////////////////// public functions ///////////////////////////////

func useTQ(c context.Context) context.Context {
	return gae.SetTQFactory(c, func(ic context.Context) gae.TaskQueue {
		tqd := cur(ic).Get(memContextTQIdx)
		var ret interface {
			gae.TQTestable
			gae.TaskQueue
		}
		switch x := tqd.(type) {
		case *taskQueueData:
			ret = &taskqueueImpl{
				dummy.TQ(),
				x,
				ic,
				curGID(ic).namespace,
			}

		case *txnTaskQueueData:
			ret = &taskqueueTxnImpl{
				dummy.TQ(),
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
	gae.TaskQueue
	*taskQueueData

	ctx context.Context
	ns  string
}

var (
	_ = gae.TaskQueue((*taskqueueImpl)(nil))
	_ = gae.TQTestable((*taskqueueImpl)(nil))
)

func (t *taskqueueImpl) addLocked(task *gae.TQTask, queueName string) (*gae.TQTask, error) {
	toSched, queueName, err := t.prepTask(t.ctx, t.ns, task, queueName)
	if err != nil {
		return nil, err
	}

	if _, ok := t.archived[queueName][toSched.Name]; ok {
		// SDK converts TOMBSTONE -> already added too
		return nil, gae.ErrTQTaskAlreadyAdded
	} else if _, ok := t.named[queueName][toSched.Name]; ok {
		return nil, gae.ErrTQTaskAlreadyAdded
	} else {
		t.named[queueName][toSched.Name] = toSched
	}

	return dupTask(toSched), nil
}

func (t *taskqueueImpl) Add(task *gae.TQTask, queueName string) (retTask *gae.TQTask, err error) {
	err = t.RunIfNotBroken(func() (err error) {
		t.Lock()
		defer t.Unlock()
		retTask, err = t.addLocked(task, queueName)
		return
	})
	return
}

func (t *taskqueueImpl) deleteLocked(task *gae.TQTask, queueName string) error {
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

func (t *taskqueueImpl) Delete(task *gae.TQTask, queueName string) error {
	return t.RunIfNotBroken(func() error {
		t.Lock()
		defer t.Unlock()
		return t.deleteLocked(task, queueName)
	})
}

func (t *taskqueueImpl) AddMulti(tasks []*gae.TQTask, queueName string) (retTasks []*gae.TQTask, err error) {
	err = t.RunIfNotBroken(func() (err error) {
		t.Lock()
		defer t.Unlock()
		retTasks, err = multi(tasks, queueName, t.addLocked)
		return
	})
	return
}

func (t *taskqueueImpl) DeleteMulti(tasks []*gae.TQTask, queueName string) error {
	return t.RunIfNotBroken(func() error {
		t.Lock()
		defer t.Unlock()

		_, err := multi(tasks, queueName,
			func(tsk *gae.TQTask, qn string) (*gae.TQTask, error) {
				return nil, t.deleteLocked(tsk, qn)
			})
		return err
	})
}

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	gae.TaskQueue
	*txnTaskQueueData

	ctx context.Context
	ns  string
}

var (
	_ = gae.TaskQueue((*taskqueueTxnImpl)(nil))
	_ = gae.TQTestable((*taskqueueTxnImpl)(nil))
)

func (t *taskqueueTxnImpl) addLocked(task *gae.TQTask, queueName string) (*gae.TQTask, error) {
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

func (t *taskqueueTxnImpl) Add(task *gae.TQTask, queueName string) (retTask *gae.TQTask, err error) {
	err = t.RunIfNotBroken(func() (err error) {
		t.Lock()
		defer t.Unlock()
		retTask, err = t.addLocked(task, queueName)
		return
	})
	return
}

func (t *taskqueueTxnImpl) AddMulti(tasks []*gae.TQTask, queueName string) (retTasks []*gae.TQTask, err error) {
	err = t.RunIfNotBroken(func() (err error) {
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

func mkName(c context.Context, cur string, queue map[string]*gae.TQTask) string {
	_, ok := queue[cur]
	for !ok && cur == "" {
		name := [500]byte{}
		for i := 0; i < 500; i++ {
			name[i] = validTaskChars[gae.GetMathRand(c).Intn(len(validTaskChars))]
		}
		cur = string(name[:])
		_, ok = queue[cur]
	}
	return cur
}

func multi(tasks []*gae.TQTask, queueName string, f func(*gae.TQTask, string) (*gae.TQTask, error)) ([]*gae.TQTask, error) {
	ret := []*gae.TQTask(nil)
	me := gae.MultiError(nil)
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

func dupTask(t *gae.TQTask) *gae.TQTask {
	ret := &gae.TQTask{}
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
		ret.RetryOptions = &gae.TQRetryOptions{}
		*ret.RetryOptions = *t.RetryOptions
	}

	return ret
}

func dupQueue(q gae.QueueData) gae.QueueData {
	r := make(gae.QueueData, len(q))
	for k, q := range q {
		r[k] = make(map[string]*gae.TQTask, len(q))
		for tn, t := range q {
			r[k][tn] = dupTask(t)
		}
	}
	return r
}
