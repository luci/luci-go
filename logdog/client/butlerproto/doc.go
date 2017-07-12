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

// Package butlerproto implements the LogDog Butler wire protocol. This protocol
// wraps Butler messages that are published to Cloud Pub/Sub for LogDog
// consumption.
//
// The protocol begins with a set of header bytes to identify and parameterize
// the remaining data, followed by the data itself.
//
// Note that the Pub/Sub layer is assumed to provide both a total length (so no
// need to length-prefix) and integrity (so no need to checksum).
//
// Header
//
// The header is a fixed four bytes which positively identify the message as a
// Butler Pub/Sub message and describe the remaining data. Variant parameters
// can use different magic numbers to identify different parameters.
//
// Two magic numbers are currently defined:
//   - 0x10 0x6d 0x06 0x00 (LogDog protocol ButlerLogBundle Raw)
//   - 0x10 0x6d 0x06 0x62 (LogDog protocol ButlerLogBundle GZip compressed)
//
// Data
//
// The data component is described by the header, and consists of all data in
// the Pub/Sub message past the last header byte.
package butlerproto
