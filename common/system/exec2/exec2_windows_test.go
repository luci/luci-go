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

package exec2

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func BenchmarkIterateChildThreads(b *testing.B) {
	nop := func(uint32) error {
		return nil
	}

	cmd := exec.Command("go.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED,
	}

	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := iterateChildThreads(uint32(cmd.Process.Pid), nop); err != nil {
			b.Fatal(cmd.Process.Pid, i, err)
		}
	}

	b.StopTimer()

	if err := cmd.Process.Kill(); err != nil {
		b.Fatal(err)
	}
}
