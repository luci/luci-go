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

// Package history implements serialization and deserilization of historical
// records used for RTS evaluation.
//
// RTS evaluation uses history files to emulate CQ behavior with a candidate
// selection strategy. Conceptually a history file is a sequence of Record
// protobuf messages,
// see https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/rts/presubmit/eval/proto/eval.proto.
// More specifically, it is a Zstd-compressed RecordIO-encoded sequence of
// Records.
//
// TODO(nodir): delete this package in favor of .jsonl.gz files.
package history
