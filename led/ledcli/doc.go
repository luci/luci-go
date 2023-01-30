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

// Package ledcli implements the subcommands for the `led` command line tool.
//
// Each file here roughly corresponds to one of the subcommands of `led`. A
// `led` subcommand manipulates a `job definition` object (a proto, see the
// `job` subpackage).
//
// The `get*` subcommands writes a job definition from some external source.
// The `launch` subcommand reads a job definition and launches it in Swarming.
// The `edit*` subcommands reads a job definition, manipulates it, then writes
//
//	the evolved job definition out.
//
// led subcommands are meant to be connected in a UNIX pipeline, starting with
// a 'get' subcommand, and ending with the 'launch' subcommand.
package ledcli
