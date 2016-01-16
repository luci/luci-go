// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package prpc (provisional RPC) implements RPC over HTTP 1.x.
//
// Like gRPC:
//  - services are defined in .proto files
//  - service implementation does not depend on pRPC.
// Unlike gRPC:
//  - pRPC supports HTTP 1.x and AppEngine 1.x.
//
// Compile service definitions
//
// Use cproto tool to compile .proto files to .go files with gRPC and pRPC support.
//   go install github.com/luci/luci-go/tools/cmd/cproto
//   cd dir/with/protos
//   cproto
//
// Server example
//
// See https://github.com/luci/luci-go/tree/master/appengine/cmd/helloworld
//
// HTTP headers
//
// pRPC server processes request HTTP headers:
//  - "X-Prpc-Timeout": specifies request timeout.
//    Value format: `\d+[HMSmun]` (regular expression), where
//     - H - hour
//     - M - minute
//     - S - second
//     - m - millisecond
//     - u - microsecond
//     - n - nanosecond
//  - "Content-Type": specifies encoding of the message in the request body.
//    See Encoding section below.
//  - "Accept": specifies response message encoding. See Encoding section below.
//  - any other headers are added to metadata.MD in the context.
//    See https://godoc.org/google.golang.org/grpc/metadata#MD
//    - If a header name has "-Bin" suffix, the value is decoded from
//      standard base64 and the suffix is trimmed.
//
// Encoding
//
// pRPC supports three protocol buffer encodings. For request and responses
// they are specified in "Content-Type" and "Accept" HTTP headers respectively.
//
//  - Binary: "application/prpc; encoding=binary"
//  - JSON:   "application/prpc; encoding=json"   or "application/json"
//  - Text:   "application/prpc; encoding=text"
//
// Discovery
//
// See "github.com/luci-go/luci/server/discovery" package.
package prpc
