package noop

import "go.chromium.org/luci/scheduler/appengine/task"

var _ task.Manager = (*TaskManager)(nil)
