// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Command led implements a 'led' binary WITHOUT support for "kitchen" based
// tasks. If you need support for kitchen based tasks, see 'infra/tools/led2'.
// Hopefully kitchen isn't long for this world and will be gone soon (~mid
// 2020).
//
// Subpackages include:
//   * ledcli        - The implementation of the CLI executable, paramaterized
//                     with an object to handle kitchen jobs.
//   * ledcmd        - Implementation of 'heavyweight' led subcommands, usually
//                     those which interact with external services.
//   * job           - The job definition which is passed between subcommands,
//                     as well as structured editing APIs.
//   * job/jobcreate - Library for generating jobs from external sources
//   * job/jobexport - Library for exporting jobs to external sinks
package main

import "go.chromium.org/luci/led/ledcli"

func main() {
	ledcli.Main(nil)
}
