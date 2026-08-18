// Copyright 2025 The LUCI Authors.
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

package stacktag_test

import (
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors/errtag/stacktag"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	short := stacktag.Tag.ApplyValue(errors.New("hi"), "short")
	long := stacktag.Tag.ApplyValue(errors.New("hi"), "longlonglong")

	merged := fmt.Errorf("%w, %w", short, long)
	assert.That(t, stacktag.Tag.ValueOrDefault(merged), should.Equal("longlonglong"))
}

var globalErr = stacktag.Capture(errors.New("hi"), 0)
var globalErr2 error

func init() {
	globalErr2 = stacktag.Capture(errors.New("hi"), 0)
}

func TestInitNoCapture(t *testing.T) {
	t.Parallel()

	assert.That(t, stacktag.Tag.ValueOrDefault(globalErr), should.Equal(""))
	assert.That(t, stacktag.Tag.ValueOrDefault(globalErr2), should.Equal(""))
	assert.That(t, stacktag.Tag.ValueOrDefault(stacktag.Capture(globalErr2, 0)), should.NotBeBlank)
}

func TestPickLongest(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("I am an error")
	err = stacktag.Tag.ApplyValue(err, "stack1")
	assert.That(t, stacktag.Tag.ValueOrDefault(err), should.Match("stack1"))

	err2 := stacktag.Tag.ApplyValue(fmt.Errorf("I am different error"), "st")

	combo := errors.Join(err, err2)
	assert.That(t, stacktag.Tag.ValueOrDefault(combo), should.Match("stack1"))

	combo = errors.Join(err, err2, stacktag.Tag.ApplyValue(fmt.Errorf("another"), "staaaaaaaaAaacccck"))
	assert.That(t, stacktag.Tag.ValueOrDefault(combo), should.Match("staaaaaaaaAaacccck"))
}
