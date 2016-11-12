// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"regexp"

	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
)

/////////////////////////////// public functions ///////////////////////////////

func useTQ(c context.Context) context.Context {
	return tq.SetRawFactory(c, func(ic context.Context) tq.RawInterface {
		memCtx, isTxn := cur(ic)
		tqd := memCtx.Get(memContextTQIdx)

		if isTxn {
			return &taskqueueTxnImpl{tqd.(*txnTaskQueueData), ic}
		}
		return &taskqueueImpl{tqd.(*taskQueueData), ic}
	})
}

//////////////////////////////// taskqueueImpl /////////////////////////////////

type taskqueueImpl struct {
	*taskQueueData

	ctx context.Context
}

var (
	_ = tq.RawInterface((*taskqueueImpl)(nil))
	_ = tq.Testable((*taskqueueImpl)(nil))
)

func (t *taskqueueImpl) addLocked(task *tq.Task, queueName string) (*tq.Task, error) {
	toSched, err := t.prepTask(t.ctx, task, queueName)
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

	return toSched.Duplicate(), nil
}

func (t *taskqueueImpl) deleteLocked(task *tq.Task, queueName string) error {
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

func (t *taskqueueImpl) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	t.Lock()
	defer t.Unlock()

	queueName, err := t.getQueueNameLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		cb(t.addLocked(task, queueName))
	}
	return nil
}

func (t *taskqueueImpl) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	t.Lock()
	defer t.Unlock()

	queueName, err := t.getQueueNameLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		cb(t.deleteLocked(task, queueName))
	}
	return nil
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
		qn, err := t.getQueueNameLocked(qn)
		if err != nil {
			cb(nil, err)
		} else {
			s := tq.Statistics{
				Tasks: len(t.named[qn]),
			}
			for _, t := range t.named[qn] {
				if s.OldestETA.IsZero() {
					s.OldestETA = t.ETA
				} else if t.ETA.Before(s.OldestETA) {
					s.OldestETA = t.ETA
				}
			}
			cb(&s, nil)
		}
	}

	return nil
}

func (t *taskqueueImpl) GetTestable() tq.Testable { return t }

/////////////////////////////// taskqueueTxnImpl ///////////////////////////////

type taskqueueTxnImpl struct {
	*txnTaskQueueData

	ctx context.Context
}

var _ interface {
	tq.RawInterface
	tq.Testable
} = (*taskqueueTxnImpl)(nil)

func (t *taskqueueTxnImpl) addLocked(task *tq.Task, queueName string) (*tq.Task, error) {
	toSched, err := t.parent.prepTask(t.ctx, task, queueName)
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

	queueName, err := t.parent.getQueueNameLocked(queueName)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		cb(t.addLocked(task, queueName))
	}
	return nil
}

func (t *taskqueueTxnImpl) DeleteMulti([]*tq.Task, string, tq.RawCB) error {
	return errors.New("taskqueue: cannot DeleteMulti from a transaction")
}

func (t *taskqueueTxnImpl) Purge(string) error {
	return errors.New("taskqueue: cannot Purge from a transaction")
}

func (t *taskqueueTxnImpl) Stats([]string, tq.RawStatsCB) error {
	return errors.New("taskqueue: cannot Stats from a transaction")
}

func (t *taskqueueTxnImpl) GetTestable() tq.Testable { return t }

////////////////////////////// private functions ///////////////////////////////

var validTaskName = regexp.MustCompile("^[0-9a-zA-Z\\-\\_]{0,500}$")

const validTaskChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"

func mkName(c context.Context, cur string, queue map[string]*tq.Task) string {
	_, ok := queue[cur]
	for !ok && cur == "" {
		name := [500]byte{}
		for i := 0; i < 500; i++ {
			name[i] = validTaskChars[mathrand.Intn(c, len(validTaskChars))]
		}
		cur = string(name[:])
		_, ok = queue[cur]
	}
	return cur
}

func dupQueue(q tq.QueueData) tq.QueueData {
	r := make(tq.QueueData, len(q))
	for k, q := range q {
		r[k] = make(map[string]*tq.Task, len(q))
		for tn, t := range q {
			r[k][tn] = t.Duplicate()
		}
	}
	return r
}
