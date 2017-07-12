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

package cancelcond

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestTimeoutCond(t *testing.T) {
	t.Parallel()

	Convey(`A timeout Cond bound to a lock`, t, func() {
		c, cancelFunc := context.WithCancel(context.Background())
		l := sync.Mutex{}
		cc := New(&l)

		Convey(`Will cancel immediately if cancelled before the Wait.`, func() {
			l.Lock()
			defer l.Unlock()

			cancelFunc()
			So(cc.Wait(c), ShouldEqual, context.Canceled)
		})

		Convey(`Will unlock if cancelled.`, func() {
			l.Lock()
			defer l.Unlock()

			go func() {
				// Reclaim the lock. This ensures that "Wait()" has yielded the lock
				// prior to signalling.
				l.Lock()
				defer l.Unlock()

				cancelFunc()
			}()

			So(cc.Wait(c), ShouldEqual, context.Canceled)
		})

		Convey(`Will behave normally if not cancelled.`, func() {
			l.Lock()
			defer l.Unlock()

			go func() {
				// Reclaim the lock. This ensures that "Wait()" has yielded the lock
				// prior to signalling.
				l.Lock()
				defer l.Unlock()
				cc.Signal()
			}()

			So(cc.Wait(c), ShouldBeNil)
		})
	})
}
