// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package descriptor contains protobuf descriptor messages,
// copied from <sysroot>/include/google/protobuf/descriptor.proto.
// It also contains utility methods.
//
// The package is separate from the package google because descriptor.proto
// explicitly specifies `option go_package = "descriptor";`
package descriptor
