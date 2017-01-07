// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeTaskQueue struct {
	RawInterface
	tasks map[string][]string
}

func (ftq *fakeTaskQueue) tasksFor(queueName string) []string { return ftq.tasks[queueName] }

func (ftq *fakeTaskQueue) AddMulti(tasks []*Task, queueName string, cb RawTaskCB) error {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
		cb(t, nil)
	}
	if ftq.tasks == nil {
		ftq.tasks = make(map[string][]string)
	}
	ftq.tasks[queueName] = append(ftq.tasks[queueName], names...)
	return nil
}

type counterFilter struct {
	add int32
}

func (cf *counterFilter) filter() RawFilter {
	return func(c context.Context, rds RawInterface) RawInterface {
		return &counterFilterInst{
			RawInterface:  rds,
			counterFilter: cf,
		}
	}
}

type counterFilterInst struct {
	RawInterface
	*counterFilter
}

func (rc *counterFilterInst) AddMulti(tasks []*Task, queueName string, cb RawTaskCB) error {
	atomic.AddInt32(&rc.add, 1)
	return rc.RawInterface.AddMulti(tasks, queueName, cb)
}

func TestAddBatch(t *testing.T) {
	t.Parallel()

	Convey("A testing task queue", t, func() {
		c := context.Background()

		ftq := fakeTaskQueue{}
		c = SetRawFactory(c, func(context.Context) RawInterface { return &ftq })

		cf := counterFilter{}
		c = AddRawFilters(c, cf.filter())

		cbCount := 0
		var cbErr error
		b := Batcher{
			Callback: func(context.Context) error {
				cbCount++
				return cbErr
			},
		}

		Convey(`Can Add a single round with no callbacks.`, func() {
			b.Size = 10
			tasks := make([]*Task, 10)
			for i := range tasks {
				tasks[i] = &Task{Name: strconv.Itoa(i)}
			}

			So(b.Add(c, "foo", tasks...), ShouldBeNil)
			So(ftq.tasksFor("foo"), ShouldResemble,
				[]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"})
			So(cf.add, ShouldEqual, 1)
			So(cbCount, ShouldEqual, 0)
		})

		Convey(`Can Add in batch.`, func() {
			b.Size = 2
			tasks := make([]*Task, 10)
			for i := range tasks {
				tasks[i] = &Task{Name: strconv.Itoa(i)}
			}

			So(b.Add(c, "foo", tasks...), ShouldBeNil)
			So(ftq.tasksFor("foo"), ShouldResemble,
				[]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"})
			So(cf.add, ShouldEqual, 5)
			So(cbCount, ShouldEqual, 4)
		})

		Convey(`Stops and returns callback errors.`, func() {
			b.Size = 1
			tasks := make([]*Task, 2)
			for i := range tasks {
				tasks[i] = &Task{Name: strconv.Itoa(i)}
			}

			cbErr = errors.New("test error")
			So(b.Add(c, "foo", tasks...), ShouldEqual, cbErr)
			So(ftq.tasksFor("foo"), ShouldResemble, []string{"0"})
			So(cf.add, ShouldEqual, 1)  // 1 add, then callback on next batch.
			So(cbCount, ShouldEqual, 1) // 1 callback, which returned error.
		})
	})
}
