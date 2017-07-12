// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lib

import (
	"os"

	"github.com/luci/luci-go/common/errors"
)

// TODO(charliea): Add timeout parameter to this function.
func AcquireExclusiveLock(path string) error {
	_, err := os.OpenFile(path, os.O_APPEND, 0666)
	if os.IsNotExist(err) {
		return errors.Reason("lock file does not exist", path).Err()
	}

	panic("implement lock acquisition")
}
