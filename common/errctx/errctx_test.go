// Copyright 2019 The LUCI Authors.
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

package errctx_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/errctx"
)

func TestCustomCancel(t *testing.T) {
	parent := context.Background()
	ctx, cancel := errctx.WithCancel(parent)
	expect := fmt.Errorf("custom error message")
	cancel(expect)
	<-ctx.Done()
	actual := ctx.Err()
	if actual != expect {
		t.Errorf("unexpected Err value, got: %s, want: %s", actual, expect)
	}
}

func TestParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, _ := errctx.WithCancel(parent)
	cancel()
	<-ctx.Done()
	actual := ctx.Err()
	expect := context.Canceled
	if actual != expect {
		t.Errorf("unexpected Err value, got: %s, want: %s", actual, expect)
	}
}

func TestWithTimeout(t *testing.T) {
	parent := context.Background()
	expect := fmt.Errorf("custom error message")
	ctx, _ := errctx.WithTimeout(parent, 10*time.Millisecond, expect)
	<-ctx.Done()
	actual := ctx.Err()
	if actual != expect {
		t.Errorf("unexpected Err value, got: %s, want: %s", actual, expect)
	}
}
