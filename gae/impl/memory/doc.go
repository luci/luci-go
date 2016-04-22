// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package memory provides an implementation of infra/gae/libs/wrapper which
// backs to local memory ONLY. This is useful for unittesting, and is also used
// for the nested-transaction filter implementation.
//
// Debug EnvVars
//
// To debug GKVLite memory access for a binary that uses this memory
// implementation, you may set the flag:
//   -luci.gae.gkvlite_trace_folder
// to `/path/to/some/folder`. Every gkvlite memory store will be assigned
// a numbered file in that folder, and all access to that store will be logged
// to that file. Setting this to "-" will cause the trace information to dump to
// stdout.
package memory
