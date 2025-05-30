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

package warmup

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWorks(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		var called []string

		Register("1", func(context.Context) error {
			called = append(called, "1")
			return nil
		})

		Register("2", func(context.Context) error {
			called = append(called, "2")
			return fmt.Errorf("OMG 1")
		})

		Register("3", func(context.Context) error {
			called = append(called, "3")
			return fmt.Errorf("OMG 2")
		})

		err := Warmup(context.Background())
		assert.Loosely(t, err.Error(), should.Equal("err[0]: OMG 1\nerr[1]: OMG 2"))
		assert.Loosely(t, called, should.Match([]string{"1", "2", "3"}))
	})
}
