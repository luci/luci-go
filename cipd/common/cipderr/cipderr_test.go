// Copyright 2022 The LUCI Authors.
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

package cipderr

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
)

func TestError(t *testing.T) {
	t.Parallel()

	Convey("Setting and getting code", t, func() {
		So(ToCode(nil), ShouldEqual, Unknown)
		So(ToCode(fmt.Errorf("old school")), ShouldEqual, Unknown)
		So(ToCode(errors.New("new")), ShouldEqual, Unknown)

		So(ToCode(errors.New("tagged", Auth)), ShouldEqual, Auth)
		So(ToCode(errors.New("tagged", Auth.WithDetails(Details{
			Package: "zzz",
		}))), ShouldEqual, Auth)

		err1 := errors.Annotate(errors.New("tagged", Auth), "deeper").Err()
		So(ToCode(err1), ShouldEqual, Auth)

		err2 := errors.Annotate(errors.New("tagged", Auth), "deeper").Tag(IO).Err()
		So(ToCode(err2), ShouldEqual, IO)

		err3 := errors.MultiError{
			fmt.Errorf("old school"),
			errors.New("1", Auth),
			errors.New("1", IO),
		}
		So(ToCode(err3), ShouldEqual, Auth)

		err4 := errors.Annotate(context.DeadlineExceeded, "blah").Err()
		So(ToCode(err4), ShouldEqual, Timeout)
	})

	Convey("Details", t, func() {
		So(ToDetails(nil), ShouldEqual, nil)
		So(ToDetails(errors.New("blah", Auth)), ShouldEqual, nil)

		d := Details{Package: "a", Version: "b"}
		So(ToDetails(errors.New("blah", Auth.WithDetails(d))), ShouldResemble, &d)

		var err error
		AttachDetails(&err, d)
		So(err, ShouldBeNil)

		err = errors.New("blah", Auth)
		AttachDetails(&err, d)
		So(ToCode(err), ShouldEqual, Auth)
		So(ToDetails(err), ShouldResemble, &d)

		err = errors.New("blah", Auth.WithDetails(Details{Package: "zzz"}))
		AttachDetails(&err, d)
		So(ToCode(err), ShouldEqual, Auth)
		So(ToDetails(err), ShouldResemble, &d)
	})
}
