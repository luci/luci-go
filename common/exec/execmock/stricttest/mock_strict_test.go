// Copyright 2026 The LUCI Authors.
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

package stricttest

import (
	"context"
	"os"
	"testing"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMain(m *testing.M) {
	execmock.Intercept(execmock.Panic)
	os.Exit(m.Run())
}

// TestUnmockedCommandPanics tests that an unmocked command panics.
func TestUnmockedCommandPanics(t *testing.T) {
	t.Parallel()

	assert.That(t, func() {
		_ = exec.Command(context.Background(), "ls").Run()
	}, should.Panic)
}
