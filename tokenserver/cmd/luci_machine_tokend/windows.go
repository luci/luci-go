// Copyright 2016 The LUCI Authors.
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

// +build windows

package main

import (
	"syscall"
	"unsafe"
)

// See https://msdn.microsoft.com/en-us/library/windows/desktop/aa365240(v=vs.85).aspx

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procMoveFileExW = kernel32.NewProc("MoveFileExW")
)

const (
	moveFileReplaceExisting = 1
	moveFileWriteThrough    = 8
)

func moveFileEx(source, target *uint16, flags uint32) error {
	ret, _, err := procMoveFileExW.Call(uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(target)), uintptr(flags))
	if ret == 0 {
		if err != nil {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

func atomicRename(source, target string) error {
	lpReplacedFileName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	lpReplacementFileName, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	return moveFileEx(lpReplacementFileName, lpReplacedFileName, moveFileReplaceExisting|moveFileWriteThrough)
}
