// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package bootstrap provides functions to bootstrap a buildbucket build.
// See "Executable" message in
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/buildbucket/proto/common.proto
// for the protocol between this package and a user-provided executable.
package bootstrap
