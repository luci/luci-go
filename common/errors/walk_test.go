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

package errors_test

import (
	stderrors "errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWalk(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing the Walk function`, t, func(t *ftt.Test) {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		t.Run(`Will not walk at all for a nil error.`, func(t *ftt.Test) {
			errors.Walk(nil, walkFn)
			assert.Loosely(t, count, should.BeZero)
		})

		t.Run(`Will fully traverse a wrapped MultiError.`, func(t *ftt.Test) {
			errors.Walk(errors.MultiError{nil, testWrap(errors.New("sup")), nil}, walkFn)
			assert.Loosely(t, count, should.Equal(4))
		})

		t.Run(`Will unwrap a Wrapped error.`, func(t *ftt.Test) {
			errors.Walk(testWrap(errors.New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(3))
		})

		t.Run(`Will short-circuit if the walk function returns false.`, func(t *ftt.Test) {
			keepWalking = false
			errors.Walk(testWrap(errors.New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})
	})

	ftt.Run(`Testing the WalkLeaves function`, t, func(t *ftt.Test) {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		t.Run(`Will not walk at all for a nil error.`, func(t *ftt.Test) {
			errors.WalkLeaves(nil, walkFn)
			assert.Loosely(t, count, should.BeZero)
		})

		t.Run(`Will visit a simple annotator error.`, func(t *ftt.Test) {
			errors.WalkLeaves(errors.Reason("sup"), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will visit a wrapped annotator error.`, func(t *ftt.Test) {
			errors.WalkLeaves(errors.Annotate(errors.Reason("sup"), "boo"), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will traverse leaves of a wrapped MultiError.`, func(t *ftt.Test) {
			errors.WalkLeaves(errors.MultiError{nil, testWrap(errors.New("sup")), errors.New("sup")}, walkFn)
			assert.Loosely(t, count, should.Equal(2))
		})

		t.Run(`Will unwrap a Wrapped error.`, func(t *ftt.Test) {
			errors.WalkLeaves(testWrap(errors.New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will short-circuit if the walk function returns false.`, func(t *ftt.Test) {
			keepWalking = false
			errors.WalkLeaves(errors.MultiError{testWrap(errors.New("sup")), errors.New("foo")}, walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})
	})
}

type intError int

func (i *intError) Is(err error) bool {
	if e, ok := err.(*intError); ok {
		return int(*i)/2 == int(*e)/2
	}
	return false
}

func (i *intError) Error() string {
	return fmt.Sprintf("%d", int(*i))
}

func TestAny(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing the Any function`, t, func(t *ftt.Test) {
		testErr := stderrors.New("test error")
		filter := func(err error) bool { return err == testErr }

		for _, err := range []error{
			nil,
			errors.Reason("error test: foo"),
			stderrors.New("other error"),
		} {
			t.Run(fmt.Sprintf(`Registers false for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, errors.Any(err, filter), should.BeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			errors.MultiError{stderrors.New("other error"), errors.MultiError{testErr, nil}},
			errors.Annotate(testErr, "error test"),
		} {
			t.Run(fmt.Sprintf(`Registers true for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, errors.Any(err, filter), should.BeTrue)
			})
		}
	})
}

func TestContains(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing the Contains function`, t, func(t *ftt.Test) {
		testErr := stderrors.New("test error")

		for _, err := range []error{
			nil,
			errors.Reason("error test: foo"),
			stderrors.New("other error"),
		} {
			t.Run(fmt.Sprintf(`Registers false for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, errors.Contains(err, testErr), should.BeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			errors.MultiError{stderrors.New("other error"), errors.MultiError{testErr, nil}},
			errors.Annotate(testErr, "error test"),
		} {
			t.Run(fmt.Sprintf(`Registers true for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, errors.Contains(err, testErr), should.BeTrue)
			})
		}

		t.Run(`Support Is`, func(t *ftt.Test) {
			e0 := intError(0)
			e1 := intError(1)
			e2 := intError(2)
			wrapped0 := testWrap(&e0)
			assert.Loosely(t, errors.Contains(wrapped0, &e0), should.BeTrue)
			assert.Loosely(t, errors.Contains(wrapped0, &e1), should.BeTrue)
			assert.Loosely(t, errors.Contains(wrapped0, &e2), should.BeFalse)
		})
	})
}

var (
	tagA = errtag.Make("TagA", true)
	tagB = errtag.Make("TagB", "val")
	tagC = errtag.Make("TagC", 123)
)

func TestParseTree(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing ParseTree and Tree.String`, t, func(t *ftt.Test) {
		t.Run(`nil error`, func(t *ftt.Test) {
			tree := errors.ParseTree(nil)
			assert.Loosely(t, tree.Err, should.BeNil)
			assert.Loosely(t, tree.String(), should.Equal("<nil>"))
		})

		t.Run(`simple untagged error`, func(t *ftt.Test) {
			err := stderrors.New("something went wrong")
			tree := errors.ParseTree(err)
			assert.Loosely(t, tree.Err, should.Equal(err))
			assert.Loosely(t, len(tree.Tags), should.BeZero)
			assert.Loosely(t, len(tree.Wraps), should.BeZero)
			assert.Loosely(t, tree.String(), should.Equal("something went wrong"))
		})

		t.Run(`single tagged error`, func(t *ftt.Test) {
			core := stderrors.New("bad input")
			err := tagA.Apply(core)
			tree := errors.ParseTree(err)
			assert.Loosely(t, tree.Err, should.Equal(err))
			assert.Loosely(t, tagA.In(tree.Err), should.BeTrue)
			assert.Loosely(t, tree.Tags, should.Resemble([]errtag.TagKey{tagA.Key()}))
			assert.Loosely(t, len(tree.Wraps), should.BeZero)
			assert.Loosely(t, tree.String(), should.Equal("bad input\n └ [TagA]"))
		})

		t.Run(`multiple tags on single node preserve application order`, func(t *ftt.Test) {
			core := stderrors.New("bad input")
			err := tagC.Apply(tagA.Apply(tagB.Apply(core)))
			tree := errors.ParseTree(err)
			assert.Loosely(t, tree.Tags, should.Resemble([]errtag.TagKey{
				tagC.Key(),
				tagA.Key(),
				tagB.Key(),
			}))
			assert.Loosely(t, tree.Err, should.Equal(err))
			assert.Loosely(t, tagA.In(tree.Err), should.BeTrue)
			assert.Loosely(t, tagB.ValueOrDefault(tree.Err), should.Equal("val"))
			assert.Loosely(t, tagC.ValueOrDefault(tree.Err), should.Equal(123))
			assert.Loosely(t, tree.String(), should.Equal("bad input\n ├ [TagC]\n ├ [TagA]\n └ [TagB]"))
		})

		t.Run(`single wrapped error`, func(t *ftt.Test) {
			inner := stderrors.New("inner error")
			outer := fmt.Errorf("outer error: %w", inner)
			tree := errors.ParseTree(outer)
			assert.Loosely(t, tree.Err, should.Equal(outer))
			assert.Loosely(t, len(tree.Wraps), should.Equal(1))
			assert.Loosely(t, tree.Wraps[0].Err, should.Equal(inner))
			assert.Loosely(t, tree.String(), should.Equal("outer error: inner error\n └ inner error"))
		})

		t.Run(`wrapped error with tags at both levels`, func(t *ftt.Test) {
			inner := tagB.Apply(stderrors.New("inner error"))
			outer := tagA.Apply(fmt.Errorf("outer error: %w", inner))
			tree := errors.ParseTree(outer)
			assert.Loosely(t, tree.Tags, should.Resemble([]errtag.TagKey{tagA.Key()}))
			assert.Loosely(t, len(tree.Wraps), should.Equal(1))
			assert.Loosely(t, tree.Err, should.Equal(outer))
			assert.Loosely(t, tagA.In(tree.Err), should.BeTrue)
			assert.Loosely(t, tree.Wraps[0].Err, should.Equal(inner))
			assert.Loosely(t, tagB.ValueOrDefault(tree.Wraps[0].Err), should.Equal("val"))
			assert.Loosely(t, tree.Wraps[0].Tags, should.Resemble([]errtag.TagKey{tagB.Key()}))
			assert.Loosely(t, tree.String(), should.Equal(`outer error: inner error
 ├ [TagA]
 └ inner error
   └ [TagB]`))
		})

		t.Run(`multi error (Unwrap []error)`, func(t *ftt.Test) {
			err1 := tagA.Apply(stderrors.New("err 1"))
			err2 := tagB.Apply(stderrors.New("err 2"))
			merr := errors.MultiError{err1, err2}
			tree := errors.ParseTree(merr)
			assert.Loosely(t, len(tree.Wraps), should.Equal(2))
			assert.Loosely(t, tree.String(), should.Equal(`╔err[0]: err 1
╚err[1]: err 2
 ├ err 1
 │ └ [TagA]
 └ err 2
   └ [TagB]`))
		})

		t.Run(`std errors.Join (Unwrap []error)`, func(t *ftt.Test) {
			err1 := tagA.Apply(stderrors.New("err 1"))
			err2 := tagB.Apply(stderrors.New("err 2"))
			jerr := stderrors.Join(err1, err2)
			tree := errors.ParseTree(jerr)
			assert.Loosely(t, len(tree.Wraps), should.Equal(2))
			assert.Loosely(t, tree.String(), should.Equal(`╔err 1
╚err 2
 ├ err 1
 │ └ [TagA]
 └ err 2
   └ [TagB]`))
		})

		t.Run(`multi error containing nil`, func(t *ftt.Test) {
			err1 := stderrors.New("err 1")
			merr := errors.MultiError{nil, err1, nil}
			tree := errors.ParseTree(merr)
			assert.Loosely(t, len(tree.Wraps), should.Equal(3))
			assert.Loosely(t, tree.Wraps[0].Err, should.BeNil)
			assert.Loosely(t, tree.Wraps[1].Err, should.Equal(err1))
			assert.Loosely(t, tree.Wraps[2].Err, should.BeNil)
			assert.Loosely(t, tree.String(), should.Equal(`err 1
 ├ <nil>
 ├ err 1
 └ <nil>`))
		})

		t.Run(`wrapped nil error`, func(t *ftt.Test) {
			wrappedNil := testWrap(nil)
			tree := errors.ParseTree(wrappedNil)
			assert.Loosely(t, len(tree.Wraps), should.Equal(1))
			assert.Loosely(t, tree.Wraps[0].Err, should.BeNil)
			assert.Loosely(t, tree.String(), should.Equal("wrapped: nil\n └ <nil>"))
		})
	})
}

func ExampleTree_String() {
	tag := errtag.Make("Tag", true)

	err := errors.Join(
		tag.Apply(tag.Apply(stderrors.New(`inner err 1 line 1
inner err 1 line 2
inner err 1 line 3`))),
		tag.Apply(stderrors.New("inner err N")),
	)

	err = tag.Apply(errors.Fmt("outer: %w", err))

	tree := errors.ParseTree(err)

	fmt.Println(tree.String())

	// Output:
	// ╔outer: inner err 1 line 1
	// ║inner err 1 line 2
	// ║inner err 1 line 3
	// ╚inner err N
	//  ├ [Tag]
	//  ├ [stacktag.Capture]
	//  └╔inner err 1 line 1
	//   ║inner err 1 line 2
	//   ║inner err 1 line 3
	//   ╚inner err N
	//    ├╔inner err 1 line 1
	//    │║inner err 1 line 2
	//    │╚inner err 1 line 3
	//    │ ├ [Tag]
	//    │ └ [Tag]
	//    └ inner err N
	//      └ [Tag]
}
