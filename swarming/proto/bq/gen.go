// Copyright 2024 The LUCI Authors.
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

// Package bqpb contains protobuf messages describing Swarming BigQuery tables.
//
// Many messages here are similar, but not identical, to messages used by RPC
// APIs, for two reasons:
//
//  1. Historical. These protos were designed with optimistic assumptions. They
//     include a lot of features that were never implemented. It is now very
//     hard to simplify them without breaking existing consumers.
//  2. BQ protos need to use `float64` instead of google.protobuf.Duration and
//     `string` with JSON instead of google.protobuf.Struct.
package bqpb

//go:generate cproto
