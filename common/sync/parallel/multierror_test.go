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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type otherMEType []error

func (o otherMEType) Error() string { return "FAIL" }

func TestMultiError(t *testing.T) {
	t.Parallel()

	Convey("MultiError works", t, func() {
		Convey("multiErrorFromErrors with errors works", func() {
			mec := make(chan error, 4)
			mec <- nil
			mec <- fmt.Errorf("first error")
			mec <- nil
			mec <- fmt.Errorf("what")
			close(mec)

			err := multiErrorFromErrors(mec)
			So(err.Error(), ShouldEqual, `first error (and 1 other error)`)
		})

		Convey("multiErrorFromErrors with nil works", func() {
			So(multiErrorFromErrors(nil), ShouldBeNil)

			c := make(chan error)
			close(c)
			So(multiErrorFromErrors(c), ShouldBeNil)

			c = make(chan error, 2)
			c <- nil
			c <- nil
			close(c)
			So(multiErrorFromErrors(c), ShouldBeNil)
		})
	})
}
