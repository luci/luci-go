// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

import (
	"google.golang.org/appengine/taskqueue"
)

// ErrTaskAlreadyAdded is the error returned when a named task is added to a
// task queue more than once.
var ErrTaskAlreadyAdded = taskqueue.ErrTaskAlreadyAdded
