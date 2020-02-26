// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package ledcli implements the subcommands for the `led` command line tool.
//
// Each file here roughly corresponds to one of the subcommands of `led`. A
// `led` subcommand manipulates a `job definition` object (a proto, see the
// `job` subpackage).
//
// The `get*` subcommands writes a job definition from some external source.
// The `launch` subcommand reads a job definition and launches it in Swarming.
// The `edit*` subcommands reads a job definition, manipulates it, then writes
//   the evolved job definition out.
//
// led subcommands are meant to be connected in a UNIX pipeline, starting with
// a 'get' subcommand, and ending with the 'launch' subcommand.
package ledcli
