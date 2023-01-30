// Copyright 2020 The LUCI Authors.
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

// Command led implements a 'led' binary WITHOUT support for "kitchen" based
// tasks. If you need support for kitchen based tasks, see 'infra/tools/led'.
//
// TODO(crbug.com/1015181) Hopefully kitchen isn't long for this world and will
// be gone soon (~mid 2020).
//
// Subpackages include:
//   - ledcli        - The implementation of the CLI executable, paramaterized
//     with an object to handle kitchen jobs.
//   - ledcmd        - Implementation of 'heavyweight' led subcommands, usually
//     those which interact with external services.
//   - job           - The job definition which is passed between subcommands,
//     as well as structured editing APIs.
//   - job/jobcreate - Library for generating jobs from external sources
//   - job/jobexport - Library for exporting jobs to external sinks
package main

import "go.chromium.org/luci/led/ledcli"

func main() {
	ledcli.Main(nil)
}
