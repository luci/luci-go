// Copyright 2023 The LUCI Authors.
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

package exec

import (
	"context"
	"os"
	"testing"

	"go.chromium.org/luci/common/exec/internal/execmockctx"
)

const selfTestString = "EXEC_TEST_SELF_CALL"

func TestExecWithoutIntercept(t *testing.T) {
	// t.Parallel() - serialized because this mocks getMockCreator.
	oldVal := getMockCreator
	getMockCreator = func(ctx context.Context) (mocker execmockctx.CreateMockInvocation, chatty bool) {
		return
	}
	t.Cleanup(func() { getMockCreator = oldVal })

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %s", err)
	}
	c := CommandContext(context.Background(), self)
	c.Env = append(c.Env, selfTestString+"=1")

	out, err := c.Output()
	if err != nil {
		t.Fatalf("c.Output failed %s", err)
	}

	if string(out) != selfTestString {
		t.Fatalf("Got bad output: %q vs %q", string(out), selfTestString)
	}
}
