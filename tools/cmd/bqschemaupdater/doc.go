// Copyright 2017 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Command bqschemaupdater accepts location and schema of a BigQuery table and
// creates or updates the table.
//
// When converting a proto message to BigQuery schema:
//
//   - each message field becomes a BigQuery field
//   - if a field has leading comments, common indentation is trimmed
//     and the result becomes the BigQuery field description
//   - if a field is of enum type, the BigQuery type is string
//     and valid values are appended to the BigQuery field description
//   - if a field is google.protobuf.Timestamp, the BigQuery type is TIMESTAMP
//   - if a field is of message type, the BigQuery type is RECORD
//     with schema corresponding to the proto field type. Recursively.
package main
