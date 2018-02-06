// Copyright 2017 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"os"
	"syscall"
)

// For reasons why this is needed:
// https://groups.google.com/d/topic/golang-dev/dS2GnwizSkk

func openSharedDelete(name string) (*os.File, error) {
	lpFileName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		lpFileName,
		uint32(syscall.GENERIC_READ),
		uint32(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE),
		nil,
		uint32(syscall.OPEN_EXISTING),
		syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}
