// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package ledcmd implements the subcommands for the `led` command line tool.
//
// Each file here corresponds to one of the subcommands of `led`. A `led`
// subcommand manipulates a `led job` object (a proto, see the `job`
// subpackage).
//
// The `get*` subcommands writes a led job from some external source.
// The `launch` subcommand reads a led job and launches it in Swarming.
// The `edit*` subcommands reads a led job , manipulates it, then writes
//   the evolved led job out.
package ledcmd
