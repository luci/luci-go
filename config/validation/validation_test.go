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

package validation

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	configpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("No errors", t, func(t *ftt.Test) {
		c := Context{Context: context.Background()}

		c.SetFile("zz")
		c.Enter("zzz")
		c.Exit()

		assert.Loosely(t, c.Finalize(), should.BeNil)
	})

	ftt.Run("One simple error", t, func(t *ftt.Test) {
		c := Context{Context: context.Background()}

		c.SetFile("file.cfg")
		c.Enter("ctx %d", 123)
		assert.That(t, c.HasPendingErrors(), should.BeFalse)

		c.Errorf("blah %s", "zzz")
		assert.That(t, c.HasPendingErrors(), should.BeTrue)
		assert.That(t, c.HasPendingWarnings(), should.BeFalse)

		err := c.Finalize()
		assert.Loosely(t, err, should.HaveType[*Error])
		assert.Loosely(t, err.Error(), should.Equal(`in "file.cfg" (ctx 123): blah zzz`))

		singleErr := err.(*Error).Errors[0]
		assert.Loosely(t, singleErr.Error(), should.Equal(`in "file.cfg" (ctx 123): blah zzz`))
		d, ok := fileTag.In(singleErr)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, d, should.Equal("file.cfg"))

		elts, ok := elementTag.In(singleErr)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, elts, should.Match([]string{"ctx 123"}))

		severity, ok := SeverityTag.In(singleErr)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, severity, should.Equal(Blocking))

		blocking := err.(*Error).WithSeverity(Blocking)
		assert.Loosely(t, blocking.(errors.MultiError), should.HaveLength(1))
		warns := err.(*Error).WithSeverity(Warning)
		assert.Loosely(t, warns, should.BeNil)

		assert.Loosely(t, err.(*Error).ToValidationResultMsgs(c.Context), should.Match([]*configpb.ValidationResult_Message{
			{
				Path:     "file.cfg",
				Severity: configpb.ValidationResult_ERROR,
				Text:     `in "file.cfg" (ctx 123): blah zzz`,
			},
		}))
	})

	ftt.Run("One simple warning", t, func(t *ftt.Test) {
		c := Context{Context: context.Background()}
		assert.That(t, c.HasPendingWarnings(), should.BeFalse)

		c.Warningf("option %q is a noop, please remove", "xyz: true")
		assert.That(t, c.HasPendingWarnings(), should.BeTrue)
		assert.That(t, c.HasPendingErrors(), should.BeFalse)

		err := c.Finalize()
		assert.Loosely(t, err, should.NotBeNil)
		singleErr := err.(*Error).Errors[0]
		assert.Loosely(t, singleErr.Error(), should.ContainSubstring(`option "xyz: true" is a noop, please remove`))

		severity, ok := SeverityTag.In(singleErr)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, severity, should.Equal(Warning))

		blocking := err.(*Error).WithSeverity(Blocking)
		assert.Loosely(t, blocking, should.BeNil)
		warns := err.(*Error).WithSeverity(Warning)
		assert.Loosely(t, warns.(errors.MultiError), should.HaveLength(1))

		assert.Loosely(t, err.(*Error).ToValidationResultMsgs(c.Context), should.Match([]*configpb.ValidationResult_Message{
			{
				Path:     "unspecified file",
				Severity: configpb.ValidationResult_WARNING,
				Text:     `in <unspecified file>: option "xyz: true" is a noop, please remove`,
			},
		}))
	})

	ftt.Run("Regular usage", t, func(t *ftt.Test) {
		c := Context{Context: context.Background()}

		c.Errorf("top %d", 1)
		c.Errorf("top %d", 2)

		c.SetFile("file_1.cfg")
		c.Errorf("f1")
		c.Errorf("f2")

		c.Enter("p %d", 1)
		c.Errorf("zzz 1")

		c.Enter("p %d", 2)
		c.Errorf("zzz 2")
		c.Exit()

		c.Warningf("zzz 3")

		c.SetFile("file_2.cfg")
		c.Errorf("zzz 4")

		err := c.Finalize()
		assert.Loosely(t, err, should.HaveType[*Error])
		assert.Loosely(t, err.Error(), should.Equal(`in <unspecified file>: top 1 (and 7 other errors)`))

		var errs []string
		for _, e := range err.(*Error).Errors {
			errs = append(errs, e.Error())
		}
		assert.Loosely(t, errs, should.Match([]string{
			`in <unspecified file>: top 1`,
			`in <unspecified file>: top 2`,
			`in "file_1.cfg": f1`,
			`in "file_1.cfg": f2`,
			`in "file_1.cfg" (p 1): zzz 1`,
			`in "file_1.cfg" (p 1 / p 2): zzz 2`,
			`in "file_1.cfg" (p 1): zzz 3`,
			`in "file_2.cfg": zzz 4`,
		}))

		blocking := err.(*Error).WithSeverity(Blocking)
		assert.Loosely(t, blocking.(errors.MultiError), should.HaveLength(7))
		warns := err.(*Error).WithSeverity(Warning)
		assert.Loosely(t, warns.(errors.MultiError), should.HaveLength(1))
	})
}
