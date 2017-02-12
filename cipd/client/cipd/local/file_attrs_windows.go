// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build windows

package local

import (
	"os"
	"syscall"
)

func setWinFileAttributes(path string, attrs WinAttrs) error {
	if attrs == 0 {
		return nil
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.SetFileAttributes(p, uint32(attrs))
}

func getWinFileAttributes(info os.FileInfo) (WinAttrs, error) {
	raw := info.Sys().(*syscall.Win32FileAttributeData)
	return WinAttrs(raw.FileAttributes) & WinAttrsAll, nil
}
