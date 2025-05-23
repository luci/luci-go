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

package parallel

import (
	"fmt"
	"sort"
	"sync"
)

// ExampleSemaphore demonstrates Semaphore's usage by processing 20 units of
// data in parallel. It protects the processing with a semaphore Locker to
// ensure that at most 5 units are processed at any given time.
func ExampleSemaphore() {
	sem := make(Semaphore, 5)

	done := make([]int, 20)
	wg := sync.WaitGroup{}
	for i := 0; i < len(done); i++ {
		wg.Add(1)
		sem.Lock()
		go func() {
			defer wg.Done()
			defer sem.Unlock()
			done[i] = i
		}()
	}

	wg.Wait()
	sort.Ints(done)
	fmt.Println("Got:", done)

	// Output: Got: [0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19]
}
