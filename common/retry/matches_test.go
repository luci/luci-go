// Copyright 2018 The LUCI Authors.
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

package retry

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMatches(t *testing.T) {
	t.Parallel()

	errSentinel := errors.New("sentinel error")

	Convey(`Using a Matches iterator`, t, func() {
		c := context.Background()

		Convey(`When matching against a sentinel error`, func() {
			it := checkInvokedIterator{}
			checkInvokedFactory := func() Iterator {
				return &it
			}

			matchesSentinelError := Matches(checkInvokedFactory, func(err error) bool { return err == errSentinel })

			Convey(`Will match the sentinel`, func() {
				err := Retry(c, matchesSentinelError, func() error {
					return errSentinel
				}, nil)
				So(err, ShouldEqual, errSentinel)
				So(it.invoked, ShouldBeTrue)
			})

			Convey(`Will not match other errors`, func() {
				var errNotSentinel = errors.New("not a sentinel error")
				err := Retry(c, matchesSentinelError, func() error {
					return errNotSentinel
				}, nil)
				So(err, ShouldEqual, errNotSentinel)
				So(it.invoked, ShouldBeFalse)
			})
		})
	})
}

type checkInvokedIterator struct {
	invoked bool
}

func (it *checkInvokedIterator) Next(_ context.Context, _ error) time.Duration {
	it.invoked = true
	return Stop
}
