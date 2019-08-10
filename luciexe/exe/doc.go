// Copyright 2019 The LUCI Authors.
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

// Package exe implements LUCI Executable protocol, documented in
// message Executable in
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/buildbucket/proto/common.proto
package exe

import (
	"context"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var defaultExe exeClient

// ErrNotInitialized is returned from the methods in this package if you use
// them before invoking Initialize.
var ErrNotInitialized = errors.New("luciexe protocol is not initialized")

// Initialize initializes the current process as a LUCI Executable and allows
// the other methods in this package to work.
//
// As part of the luciexe protocol, this consumes all of the data on stdin.
//
// NOTE: Your program SHOULD call `Close()` before exiting, or there's the
// possibility of data loss.
func Initialize(ctx context.Context) error { return defaultExe.initialize(ctx) }

// GetInputBuild returns a copy of the original Build message as it was passed
// to this executable.
//
// Panics if Initialize was not called successfully.
func GetInputBuild() *pb.Build { return defaultExe.getInputBuild() }

// WriteBuild sends a new version of Build message.
//
// This method is serialized, so it may only be called once at a time.
//
// Panics if Initialize was not called successfully.
func WriteBuild(ctx context.Context, build *pb.Build) error {
	return defaultExe.writeBuild(ctx, build)
}

// Close flushes all builds written with WriteBuild and closes down the client
// connection.
//
// Failing to call this before program termination may result in lost
// WriteBuilds.
//
// Panics if Initialize was not called successfully.
func Close(ctx context.Context) error {
	return defaultExe.closeBuildStream(ctx)
}
