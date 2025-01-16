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

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package memory

import (
	"math"

	"golang.org/x/sys/unix"

	"go.chromium.org/luci/common/errors"
)

func totalSystemMemoryBytes() (uint64, error) {
	ret, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, errors.Annotate(err, "memory.TotalSystemMemoryMB").Err()
	}

	// This annoyingly returns the data as an int64_t, not uint64_t so we have to
	// check if the returned value exceeds MaxInt64 (i.e. is actually a negative
	// int64 rather than just a really large uint64)
	if ret > math.MaxInt64 {
		return 0, errors.Reason("memory.TotalSystemMemoryMB: sysctl: hw.memsize: returned negative value: %d", int64(ret)).Err()
	}
	return ret, nil
}
