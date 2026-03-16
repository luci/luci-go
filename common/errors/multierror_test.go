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

package errors

import (
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMultiError(t *testing.T) {
	t.Parallel()
	t.Run("works", func(t *testing.T) {
		var me error = MultiError{errors.New("hello"), errors.New("bob")}
		assert.That(t, me, should.ErrLikeString("err[0]: hello\nerr[1]: bob"))
	})

	t.Run("compatible with errors.Is and errors.As", func(t *testing.T) {
		inner := errors.New("hello")
		annotated := Annotate(inner, "annotated err").Err()
		var me error = MultiError{annotated, fmt.Errorf("bob")}
		assert.That(t, me, should.ErrLikeError(inner))
		assert.That(t, me, should.ErrLikeString("annotated err"))
	})
}

func TestUpstreamErrors(t *testing.T) {
	t.Parallel()

	ftt.Run("Test MultiError", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			me := MultiError(nil)
			assert.Loosely(t, me.Error(), should.Equal("(0 errors)"))
			t.Run("single", func(t *ftt.Test) {
				assert.Loosely(t, SingleError(error(me)), should.BeNil)
			})
		})
		t.Run("one", func(t *ftt.Test) {
			me := MultiError{errors.New("sup")}
			assert.Loosely(t, me.Error(), should.Equal("sup"))
		})
		t.Run("two", func(t *ftt.Test) {
			me := MultiError{errors.New("sup"), errors.New("what")}
			assert.Loosely(t, me.Error(), should.Equal("err[0]: sup\nerr[1]: what"))
		})
		t.Run("more", func(t *ftt.Test) {
			me := MultiError{errors.New("sup"), errors.New("what"), errors.New("nerds")}
			assert.Loosely(t, me.Error(), should.Equal("err[0]: sup\nerr[1]: what\nerr[2]: nerds"))

			t.Run("single", func(t *ftt.Test) {
				assert.Loosely(t, SingleError(error(me)), should.Resemble(errors.New("sup")))
			})
		})
		t.Run("Error with nil", func(t *ftt.Test) {
			me := MultiError{
				errors.New("1"),
				nil,
				errors.New("3"),
			}

			assert.Loosely(t, me.Error(), should.Equal(
				"err[0]: 1\n"+
					// 1 is nil, so it's omitted
					"err[2]: 3"))
		})

		me20 := func() MultiError {
			var e []error
			for i := range 20 {
				e = append(e, errors.New(fmt.Sprint(i+1)))
			}
			return MultiError(e)
		}

		t.Run("max non-nil", func(t *ftt.Test) {
			me := me20()
			assert.Loosely(t, me.Error(), should.Equal(
				"err[0]: 1\n"+
					"err[1]: 2\n"+
					"err[2]: 3\n"+
					"err[3]: 4\n"+
					"err[4]: 5\n"+
					"err[5]: 6\n"+
					"err[6]: 7\n"+
					"err[7]: 8\n"+
					"err[8]: 9\n"+
					"err[9]: 10\n"+
					"err[10]: 11\n"+
					"err[11]: 12\n"+
					"err[12]: 13\n"+
					"err[13]: 14\n"+
					"err[14]: 15\n"+
					"err[15]: 16\n"+
					"err[16]: 17\n"+
					"err[17]: 18\n"+
					"err[18]: 19\n"+
					"err[19]: 20"))
		})

		t.Run("max nil", func(t *ftt.Test) {
			me := me20()
			me[5] = nil
			me.MaybeAdd(errors.New("new"))
			assert.Loosely(t, me.Error(), should.Equal(
				"err[0]: 1\n"+
					"err[1]: 2\n"+
					"err[2]: 3\n"+
					"err[3]: 4\n"+
					"err[4]: 5\n"+
					// 5 is omitted since it's nil
					"err[6]: 7\n"+
					"err[7]: 8\n"+
					"err[8]: 9\n"+
					"err[9]: 10\n"+
					"err[10]: 11\n"+
					"err[11]: 12\n"+
					"err[12]: 13\n"+
					"err[13]: 14\n"+
					"err[14]: 15\n"+
					"err[15]: 16\n"+
					"err[16]: 17\n"+
					"err[17]: 18\n"+
					"err[18]: 19\n"+
					"err[19]: 20\n"+
					"err[20]: new"))
		})

		t.Run("overflow non-nil", func(t *ftt.Test) {
			me := me20()

			me.MaybeAdd(errors.New("overflow"))
			me.MaybeAdd(errors.New("overflow"))
			me.MaybeAdd(errors.New("overflow"))
			assert.Loosely(t, me.Error(), should.Equal(
				"err[0]: 1\n"+
					"err[1]: 2\n"+
					"err[2]: 3\n"+
					"err[3]: 4\n"+
					"err[4]: 5\n"+
					"err[5]: 6\n"+
					"err[6]: 7\n"+
					"err[7]: 8\n"+
					"err[8]: 9\n"+
					"err[9]: 10\n"+
					"err[10]: 11\n"+
					"err[11]: 12\n"+
					"err[12]: 13\n"+
					"err[13]: 14\n"+
					"err[14]: 15\n"+
					"err[15]: 16\n"+
					"err[16]: 17\n"+
					"err[17]: 18\n"+
					"err[18]: 19\n"+
					"err[19]: 20\n"+
					"err[20:23] <omitted>"))

			assert.Loosely(t, me[20:23].Error(), should.Equal(
				"err[0]: overflow\n"+
					"err[1]: overflow\n"+
					"err[2]: overflow"))
		})

		t.Run("overflow nil", func(t *ftt.Test) {
			me := me20()

			me[5] = nil

			me.MaybeAdd(errors.New("new"))
			me.MaybeAdd(errors.New("overflow"))
			me.MaybeAdd(errors.New("overflow"))
			me.MaybeAdd(errors.New("overflow"))
			assert.Loosely(t, me.Error(), should.Equal(
				"err[0]: 1\n"+
					"err[1]: 2\n"+
					"err[2]: 3\n"+
					"err[3]: 4\n"+
					"err[4]: 5\n"+
					// 5 is nil
					"err[6]: 7\n"+
					"err[7]: 8\n"+
					"err[8]: 9\n"+
					"err[9]: 10\n"+
					"err[10]: 11\n"+
					"err[11]: 12\n"+
					"err[12]: 13\n"+
					"err[13]: 14\n"+
					"err[14]: 15\n"+
					"err[15]: 16\n"+
					"err[16]: 17\n"+
					"err[17]: 18\n"+
					"err[18]: 19\n"+
					"err[19]: 20\n"+
					"err[20]: new\n"+
					"err[21:24] <omitted>"))

			t.Run("omitted nil", func(t *ftt.Test) {
				me[21] = nil

				assert.Loosely(t, me.Error(), should.Equal(
					"err[0]: 1\n"+
						"err[1]: 2\n"+
						"err[2]: 3\n"+
						"err[3]: 4\n"+
						"err[4]: 5\n"+
						// 5 is nil
						"err[6]: 7\n"+
						"err[7]: 8\n"+
						"err[8]: 9\n"+
						"err[9]: 10\n"+
						"err[10]: 11\n"+
						"err[11]: 12\n"+
						"err[12]: 13\n"+
						"err[13]: 14\n"+
						"err[14]: 15\n"+
						"err[15]: 16\n"+
						"err[16]: 17\n"+
						"err[17]: 18\n"+
						"err[18]: 19\n"+
						"err[19]: 20\n"+
						"err[20]: new\n"+
						"err[21:24] <omitted 2 non-nil errors>"))
			})
		})
	})

	ftt.Run("MaybeAdd", t, func(t *ftt.Test) {
		me := MultiError(nil)

		t.Run("nil", func(t *ftt.Test) {
			me.MaybeAdd(nil)
			assert.Loosely(t, me, should.HaveLength(0))
			assert.That(t, me == nil, should.BeTrue)
		})

		t.Run("thing", func(t *ftt.Test) {
			me.MaybeAdd(errors.New("sup"))
			assert.Loosely(t, me, should.HaveLength(1))
			assert.Loosely(t, error(me), should.NotBeNilInterface)

			me.MaybeAdd(errors.New("what"))
			assert.Loosely(t, me, should.HaveLength(2))
			assert.Loosely(t, error(me), should.NotBeNilInterface)
		})
	})

	ftt.Run("AsError", t, func(t *ftt.Test) {
		var me MultiError
		assert.Loosely(t, me == nil, should.BeTrue)

		var err error
		err = me // nolint:ineffassign

		// Unfortunately Go has many nil's :(
		//   So(err == nil, ShouldBeTrue)
		// Note that `ShouldBeNil` won't cut it, since it 'sees through' interfaces.

		// However!
		err = me.AsError()
		assert.Loosely(t, err == nil, should.BeTrue)
	})

	ftt.Run("SingleError passes through", t, func(t *ftt.Test) {
		e := errors.New("unique")
		assert.Loosely(t, SingleError(e), should.Equal(e))
	})
}

func TestFlatten(t *testing.T) {
	t.Parallel()

	ftt.Run("Flatten works", t, func(t *ftt.Test) {
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, Flatten(MultiError{nil, nil, MultiError{nil, nil, nil}}), should.BeNil)
		})

		t.Run("2-dim", func(t *ftt.Test) {
			oneErr := errors.New("1")
			twoErr := errors.New("2")
			assert.Loosely(t, Flatten(MultiError{nil, oneErr, nil, MultiError{nil, twoErr, nil}}),
				should.ErrLike(MultiError{oneErr, twoErr}))
		})

		t.Run("Doesn't unwrap", func(t *ftt.Test) {
			ann := Annotate(MultiError{nil, nil, nil}, "don't do this").Err()
			twoErr := errors.New("2")
			merr, yup := Flatten(MultiError{nil, ann, nil, MultiError{nil, twoErr, nil}}).(MultiError)
			assert.Loosely(t, yup, should.BeTrue)
			assert.Loosely(t, len(merr), should.Equal(2))
			assert.Loosely(t, merr, should.ErrLike(MultiError{ann, twoErr}))
		})
	})
}

func TestAppend(t *testing.T) {
	t.Parallel()
	ftt.Run("Test Append function", t, func(t *ftt.Test) {
		t.Run("combine empty", func(t *ftt.Test) {
			assert.Loosely(t, Append(), should.BeNil)
		})
		t.Run("more intricate empty cases", func(t *ftt.Test) {
			assert.Loosely(t, Append(Append()), should.BeNil)
			assert.Loosely(t, Append(nil), should.BeNil)
			assert.Loosely(t, Append(Append(Append()), Append(), nil, Append(nil, nil)), should.BeNil)
		})
		t.Run("singleton physical equality", func(t *ftt.Test) {
			e := fmt.Errorf("f59031c1-3d8d-47c4-8cff-b2b5d67ce7e7")
			assert.Loosely(t, e, should.Equal(Append(e)))
			assert.Loosely(t, e, should.Equal(Append(Append(e))))
		})
		t.Run("doubleton physical equality", func(t *ftt.Test) {
			e := fmt.Errorf("f59031c1-3d8d-47c4-8cff-b2b5d67ce7e7")
			assert.Loosely(t, Append(e, e).(MultiError)[0], should.Equal(e))
		})
		t.Run("doubleton physical equality with nils", func(t *ftt.Test) {
			e := fmt.Errorf("2d2a3939-e185-4210-9060-0cb0fdab42be")
			assert.Loosely(t, Append(nil, e, e, nil).(MultiError)[0], should.Equal(e))
		})
	})
}
