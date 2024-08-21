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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestError(t *testing.T) {
	t.Parallel()

	ftt.Run("Setting and getting code", t, func(t *ftt.Test) {
		assert.Loosely(t, ToCode(nil), should.Equal(Unknown))
		assert.Loosely(t, ToCode(fmt.Errorf("old school")), should.Equal(Unknown))
		assert.Loosely(t, ToCode(errors.New("new")), should.Equal(Unknown))

		assert.Loosely(t, ToCode(errors.New("tagged", Auth)), should.Equal(Auth))
		assert.Loosely(t, ToCode(errors.New("tagged", Auth.WithDetails(Details{
			Package: "zzz",
		}))), should.Equal(Auth))

		err1 := errors.Annotate(errors.New("tagged", Auth), "deeper").Err()
		assert.Loosely(t, ToCode(err1), should.Equal(Auth))

		err2 := errors.Annotate(errors.New("tagged", Auth), "deeper").Tag(IO).Err()
		assert.Loosely(t, ToCode(err2), should.Equal(IO))

		err3 := errors.MultiError{
			fmt.Errorf("old school"),
			errors.New("1", Auth),
			errors.New("1", IO),
		}
		assert.Loosely(t, ToCode(err3), should.Equal(Auth))

		err4 := errors.Annotate(context.DeadlineExceeded, "blah").Err()
		assert.Loosely(t, ToCode(err4), should.Equal(Timeout))
	})

	ftt.Run("Details", t, func(t *ftt.Test) {
		assert.Loosely(t, ToDetails(nil), should.BeNil)
		assert.Loosely(t, ToDetails(errors.New("blah", Auth)), should.BeNil)

		d := Details{Package: "a", Version: "b"}
		assert.Loosely(t, ToDetails(errors.New("blah", Auth.WithDetails(d))), should.Resemble(&d))

		var err error
		AttachDetails(&err, d)
		assert.Loosely(t, err, should.BeNil)

		err = errors.New("blah", Auth)
		AttachDetails(&err, d)
		assert.Loosely(t, ToCode(err), should.Equal(Auth))
		assert.Loosely(t, ToDetails(err), should.Resemble(&d))

		err = errors.New("blah", Auth.WithDetails(Details{Package: "zzz"}))
		AttachDetails(&err, d)
		assert.Loosely(t, ToCode(err), should.Equal(Auth))
		assert.Loosely(t, ToDetails(err), should.Resemble(&d))
	})
}
