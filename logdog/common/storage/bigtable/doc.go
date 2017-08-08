// Copyright 2015 The LUCI Authors.
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

// Package bigtable provides an implementation of the Storage interface backed
// by Google Cloud Platform's BigTable.
//
// Intermediate Log Table
//
// The Intermediate Log Table stores LogEntry protobufs that have been ingested,
// but haven't yet been archived. It is a tall table whose rows are keyed off
// of the log's (Path,Stream-Index) in that order.
//
// Each entry in the table will contain the following schema:
//   - Column Family "log"
//     - Column "data": the LogEntry raw protobuf data. Soft size limit of ~1MB.
//
// The log path is the composite of the log's (Prefix, Name) properties. Logs
// belonging to the same stream will share the same path, so they will be
// clustered together and suitable for efficient iteration. Immediately
// following the path will be the log's stream index.
//
//   [            20 bytes          ]     ~    [       1-5 bytes      ]
//    B64(SHA256(Path(Prefix, Name)))  + '~' + HEX(cmpbin(StreamIndex))
//
// As there is no (technical) size constraint to either the Prefix or Name
// values, they will both be hashed using SHA256 to produce a unique key
// representing that specific log stream.
//
// This allows a key to be generated representing "immediately after the row"
// by appending two '~' characters to the base hash. Since the second '~'
// character is always greater than any HEX(cmpbin(*)) value, this will
// effectively upper-bound the row.
//
// "cmpbin" (go.chromium.org/luci/common/cmpbin) will be used to format the
// stream index. It is a variable-width number encoding scheme that offers the
// guarantee that byte-sorted encoded numbers will maintain the same order as
// the numbers themselves.
package bigtable
