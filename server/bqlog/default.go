// Copyright 2021 The LUCI Authors.
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

package bqlog

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Default is a bundler installed into the server when using NewModule or
// NewModuleFromFlags.
//
// The module takes care of configuring this bundler based on the server
// environment and module's options.
//
// You still need to register your types in it using RegisterSink.
var Default Bundler

// RegisterSink tells the bundler where and how to log messages of some
// concrete proto type.
//
// There can currently be only one sink per proto message type. Must be called
// before the bundler is running. Can be called during the init() time.
func RegisterSink(sink Sink) {
	Default.RegisterSink(sink)
}

// Log asynchronously logs the given message to a BQ table associated with
// the message type via a prior RegisterSink call.
//
// This is a best effort operation (and thus returns no error).
//
// Messages are dropped when:
//   - Writes to BigQuery are failing with a fatal error:
//   - The table doesn't exist.
//   - The table has an incompatible schema.
//   - The server account has no permission to write to the table.
//   - Etc.
//   - The server crashes before it manages to flush buffered logs.
//   - The internal flush buffer is full (per MaxLiveSizeBytes).
//
// In case of transient write errors messages may be duplicated.
//
// Panics if `m` was not registered via RegisterSink or if the bundler is not
// running yet.
func Log(ctx context.Context, m proto.Message) {
	Default.Log(ctx, m)
}
