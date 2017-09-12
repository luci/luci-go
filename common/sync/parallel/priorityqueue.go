// Copyright 2017 The LUCI Authors.
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

package parallel

import (
	"container/heap"
	"sync/atomic"
)

// PriorityWork is a unit of work accepted by PriorityQueue.
type PriorityWork struct {
	// Priority specifies the order of F execution.
	// The higher the value, the sooner F will be executed.
	Priority int
	// F is the function to execute to perform work.
	F func() error
}

//type PriorityQueue struct {
//	candidateC chan PriorityWork
//	stopped chan struct{}
//}
//
//func (q *PriorityQueue) Init(workers int) {
//	q.candidateC = make(chan PriorityWork)
//	q.stopped = make(chan struct{})
//}
//
//func (q *PriorityQueue) Add(f func() error, priority int) {
//	q.candidateC <- PriorityWork{priority, f}
//}
//
//func (q *PriorityQueue) Stop() {
//	close(q.stopped)
//}
//
//// PriorityQueue works like WorkPool, but schedules work in the order of priority.
////
//// PriorityQueue consumes items from the work channel as soon as they are sent and puts them to a priority queue.
//// As soon as there is an available worker, starts the highest priority item, if any.
////
//// If gen closes work, PriorityQueue stops starting new work even if there is
//// low priority work remaining.
//func (q *PriorityQueue) Run(workers int) error {
//	if workers <= 0 {
//		panic("workers must be positive") // no jerks
//	}
//	chosen := make(chan func() error) // the work that we've decided to do
//	errC := make(chan error)
//	go func() {
//		// do the chosen work
//		errC <- WorkPool(workers, func(work chan<- func() error) {
//			for f := range chosen {
//				work <- f
//			}
//		})
//	}()
//
//	var candidates workHeap
//	currentWorkItems := 0                      // number of work items currently being processed
//	workerDone := make(chan struct{}, workers) // used to signal that a worker is done
//	for {
//		select {
//		case c, ok := <-q.candidateC:
//			heap.Push(&candidates, c)
//		case <-q.stopped:
//			// Stop scheduling new work, but wait till the already-scheduled-work is done.
//			close(chosen)
//			// The workerDone is buffered, so no need to drain it
//			return <-errC
//
//		case <-workerDone:
//			currentWorkItems--
//		}
//		// consider choosing work
//		for currentWorkItems < workers && len(candidates) > 0 {
//			pw := heap.Pop(&candidates).(PriorityWork)
//			currentWorkItems++
//			chosen <- func() error {
//				defer func() {
//					workerDone <- struct{}{}
//				}()
//				return pw.F()
//			}
//		}
//	}
//}

// PriorityQueue works like WorkPool, but schedules work in the order of priority.
//
// PriorityQueue consumes items from the work channel as soon as they are sent and puts them to a priority queue.
// As soon as there is an available worker, starts the highest priority item, if any.
//
// If gen returns false, PriorityQueue stops starting new work even if there is
// low priority work remaining in the queue.
func PriorityQueue(workers int, gen func(work chan<- PriorityWork) bool) error {
	if workers <= 0 {
		panic("workers must be positive") // no jerks
	}
	chosen := make(chan func() error) // the work that we've decided to do
	errC := make(chan error)
	go func() {
		// do the chosen work
		errC <- WorkPool(workers, func(work chan<- func() error) {
			for f := range chosen {
				work <- f
			}
		})
	}()

	// Start generating work candidates.
	stop := int32(0)
	candidateC := make(chan PriorityWork)
	go func() {
		defer close(candidateC)
		if !gen(candidateC) {
			atomic.StoreInt32(&stop, 1)
		}
	}()

	var candidates workHeap
	currentWorkItems := 0                      // number of work items currently being processed
	workerDone := make(chan struct{}, workers) // used to signal that a worker is done
	for {
		select {
		case c, ok := <-candidateC:
			switch {
			case ok:
				heap.Push(&candidates, c)
			case atomic.LoadInt32(&stop) == 1:
				// Stop scheduling new work, but wait till the already-scheduled-work is done.
				close(chosen)
				// The workerDone is buffered, so no need to drain it
				return <-errC
			default:
				// consume all candidates in order
				// stop listening to candicateC
				candidateC = nil
			}
		case <-workerDone:
			currentWorkItems--
		}
		// consider choosing work
		for currentWorkItems < workers && len(candidates) > 0 {
			pw := heap.Pop(&candidates).(PriorityWork)
			currentWorkItems++
			chosen <- func() error {
				defer func() {
					workerDone <- struct{}{}
				}()
				return pw.F()
			}
		}
		if len(candidates) == 0 && candidateC == nil {
			// no current or future work
			close(chosen)
			return <-errC
		}
	}
}

type workHeap []PriorityWork

func (h *workHeap) Less(i, j int) bool {
	return (*h)[i].Priority > (*h)[j].Priority
}

func (h *workHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *workHeap) Len() int {
	return len(*h)
}

func (h *workHeap) Pop() (v interface{}) {
	*h, v = (*h)[:h.Len()-1], (*h)[h.Len()-1]
	return
}

func (h *workHeap) Push(v interface{}) {
	*h = append(*h, v.(PriorityWork))
}
