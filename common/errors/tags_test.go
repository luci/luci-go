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

package errors

import (
	stdErr "errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type customInt int
type customIntTag struct{ Key TagKey }

func (t customIntTag) With(i customInt) TagValue { return TagValue{t.Key, i} }
func (t customIntTag) In(err error) (v customInt, ok bool) {
	d, ok := TagValueIn(t.Key, err)
	if ok {
		v = d.(customInt)
	}
	return
}

var aCustomIntTag = customIntTag{NewTagKey("errors.testing custom int tag")}

type stringTag struct{ Key TagKey }

func (t stringTag) With(s string) TagValue { return TagValue{t.Key, s} }
func (t stringTag) In(err error) (v string, ok bool) {
	d, ok := TagValueIn(t.Key, err)
	if ok {
		v = d.(string)
	}
	return
}

var aStringTag = stringTag{NewTagKey("errors.testing string tag")}

func TestTags(t *testing.T) {
	t.Parallel()

	aBoolTag := BoolTag{NewTagKey("errors.testing tag")}

	ftt.Run("Tags", t, func(t *ftt.Test) {
		t.Run(`have unique tagKey values`, func(t *ftt.Test) {
			tagSet := map[TagKey]struct{}{}
			tagSet[aBoolTag.Key] = struct{}{}
			tagSet[aStringTag.Key] = struct{}{}
			tagSet[aCustomIntTag.Key] = struct{}{}
			assert.Loosely(t, tagSet, should.HaveLength(3))
		})

		t.Run(`can be applied to errors`, func(t *ftt.Test) {
			t.Run(`at creation time`, func(t *ftt.Test) {
				err := New("I am an error", aBoolTag, aStringTag.With("hi"))
				assert.Loosely(t, aBoolTag.In(err), should.BeTrue)
				d, ok := aStringTag.In(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, d, should.Equal("hi"))

				_, ok = aCustomIntTag.In(err)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run(`added to existing errors`, func(t *ftt.Test) {
				err := New("I am an error")
				err2 := aCustomIntTag.With(236).Apply(err)
				err2 = aBoolTag.Apply(err2)

				d, ok := aCustomIntTag.In(err2)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, d, should.Equal(customInt(236)))
				assert.Loosely(t, aBoolTag.In(err2), should.BeTrue)

				_, ok = aCustomIntTag.In(err)
				assert.Loosely(t, ok, should.BeFalse)
				assert.Loosely(t, aBoolTag.In(err), should.BeFalse)
			})

			t.Run(`added to stdlib errors`, func(t *ftt.Test) {
				err := stdErr.New("I am an error")
				err2 := aStringTag.With("hi").Apply(err)

				d, ok := aStringTag.In(err2)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, d, should.Equal("hi"))

				_, ok = aStringTag.In(err)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run(`multiple applications has the last one win`, func(t *ftt.Test) {
				err := New("I am an error")
				err = aStringTag.With("hi").Apply(err)
				err = aStringTag.With("there").Apply(err)
				err = aStringTag.With("winner").Apply(err)

				d, ok := aStringTag.In(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, d, should.Equal("winner"))

				t.Run(`muliterrors are first to last`, func(t *ftt.Test) {
					err = NewMultiError(
						New("a", aStringTag.With("hi"), aBoolTag),
						New("b", aCustomIntTag.With(10), aStringTag.With("no")),
						New("c", aCustomIntTag.With(20), aStringTag.With("nopers")),
					)

					assert.Loosely(t, aBoolTag.In(err), should.BeTrue)

					d, ok := aStringTag.In(err)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, d, should.Equal("hi"))

					ci, ok := aCustomIntTag.In(err)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, ci, should.Equal(customInt(10)))

					t.Run(`and all the correct values show up with GetTags`, func(t *ftt.Test) {
						tags := GetTags(err)
						assert.Loosely(t, tags, should.ContainKey(aStringTag.Key))
						assert.Loosely(t, tags, should.ContainKey(aBoolTag.Key))

						assert.Loosely(t, tags[aCustomIntTag.Key], should.Equal(10))
						assert.Loosely(t, tags[aStringTag.Key], should.Equal("hi"))
						assert.Loosely(t, tags[aBoolTag.Key], should.Equal(true))
					})
				})
			})
		})
	})
}
