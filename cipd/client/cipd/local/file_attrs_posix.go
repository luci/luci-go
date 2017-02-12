// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !windows

package local

import "os"

func setWinFileAttributes(path string, attrs WinAttrs) error {
	return nil
}

func getWinFileAttributes(info os.FileInfo) (WinAttrs, error) {
	return 0, nil
}
