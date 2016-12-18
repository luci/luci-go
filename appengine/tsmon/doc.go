// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package tsmon adapts common/tsmon library to GAE environment.
//
// It configures tsmon state with a monitor and store suitable for GAE
// environment and controls when metric flushes happen.
//
// Timeseries metrics are gathered automatically by the tsmon middleware and
// staged in memory for export. Periodically, an unlucky single handler will
// be chosen to perform this export at the end of its operation.
//
// A cron task MUST also be installed if metrics are enabled. The task assigns
// and manages the task number assignments for active instances. If this cron
// task is not installed, instances will not get IDs and will be unable to send
// metrics. The cron task should be configured to hit:
// "/internal/cron/ts_mon/housekeeping" every minute.
package tsmon
