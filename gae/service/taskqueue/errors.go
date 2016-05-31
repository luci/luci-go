// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

import (
	"google.golang.org/appengine/taskqueue"
)

// ErrTaskAlreadyAdded is the error returned when a named task is added to a
// task queue more than once.
var ErrTaskAlreadyAdded = taskqueue.ErrTaskAlreadyAdded
