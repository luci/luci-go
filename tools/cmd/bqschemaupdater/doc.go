// Copyright 2018 The LUCI Authors.
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

// Command bqschemaupdater accepts location and schema of a BigQuery table and
// creates or updates the table.
//
// When converting a proto message to BigQuery schema, in the order of
// precedence:
//
//   - one message field becomes at most one BigQuery field
//   - if a field has leading comments, common indentation is trimmed
//     and the result becomes the BigQuery field description
//   - if a field is of enum type, the BigQuery type is string
//     and valid values are appended to the BigQuery field description
//   - if a field is google.protobuf.Duration, the BigQuery type is FLOAT64
//   - if a field is google.protobuf.Timestamp, the BigQuery type is TIMESTAMP
//   - if a field is google.protobuf.Struct, is is persisted as a JSONPB string.
//   - if a field is of message type, the BigQuery type is RECORD
//     with schema corresponding to the proto field type, recursively.
//     However, if the resulting RECORD schema is empty, the field is omitted.
package main
