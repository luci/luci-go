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

package memory

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"go.chromium.org/luci/common/errors"
)

type memoryStatusEX struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

func totalSystemMemoryBytes() (uint64, error) {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	proc := dll.NewProc("GlobalMemoryStatusEx")

	var msx memoryStatusEX
	msx.dwLength = uint32(unsafe.Sizeof(memoryStatusEX{}))
	ok, _, err := proc.Call(uintptr(unsafe.Pointer(&msx)))
	if ok == 0 {
		return 0, errors.Annotate(err, "memory.TotalSystemMemoryMB").Err()
	}
	return msx.ullTotalPhys, nil
}
