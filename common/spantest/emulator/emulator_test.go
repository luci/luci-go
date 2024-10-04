// Copyright 2024 The LUCI Authors.
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

package emulator

import (
	"context"
	"os"
	"testing"

	"go.chromium.org/luci/common/logging/gologger"
)

func TestEmulator(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip()
	}

	ctx := gologger.StdConfig.Use(context.Background())

	emu, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer emu.Stop()

	t.Logf("gRPC Address %q", emu.GrpcAddr())
}
