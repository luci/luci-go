// Copyright 2016 The LUCI Authors.
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

// Package prpc (provisional RPC) implements an RPC client over HTTP 1.x.
//
// Like gRPC:
//   - services are defined in .proto files
//   - service implementation does not depend on pRPC.
//
// Unlike gRPC:
//   - supports HTTP 1.x and AppEngine
//   - does not support streams.
//
// It is similar in purpose to some other gRPC-compatible HTTP protocols, but
// predates them or makes different design choices:
//   - https://github.com/grpc/grpc-web: pRPC avoids a proxy.
//   - https://connectrpc.com/: pRPC predates it by multiple years.
//
// # Compile service definitions
//
// Use cproto tool to compile .proto files to .go files with gRPC and pRPC
// support. It runs protoc and does some additional postprocessing.
//
//	//go:generate cproto
//
// Install cproto:
//
//	go install go.chromium.org/luci/grpc/cmd/cproto
//
// # Protocol
//
// # v1.5
//
// v1.5 is small, backward-compatible amendment to v1.4 that allows clients to
// advertise the maximum response size they are willing to deserialize.
//
// A client MAY send "X-Prpc-Max-Response-Size" request header with a positive
// decimal number of bytes specifying the maximum uncompressed response size
// the client is willing to deserialize. The server MAY use it to discard
// responses larger than that size. If the server discards a response, it
// replies with UNAVAILABLE gRPC status with luci.prpc.ErrorDetails message as
// google.rpc.Status detail (see v1.2 section below for how it is encoded). This
// error proto contains ResponseTooBig as the realized oneof "error"
// alternative.
//
// The response size is calculated as a length of the serialized response
// message using the negotiated response encoding (i.e. if the client asked for
// a JSON response, the response size is the length of the JSON blob with the
// serialized response message). Length of metadata or response compression
// (if any) have no effect on this check.
//
// # v1.4
//
// v1.4 hides some leaking HTTP v1 transport implementation details from gRPC
// services and clients by stopping exposing values of the following headers in
// metadata.MD: "Accept", "Accept-Encoding", "Content-Encoding",
// "Content-Length", "Content-Type", "X-Content-Type-Options", all "X-Prpc-*"
// headers.
//
// Note that "X-Prpc-Grpc-Timeout", "X-Prpc-Status-Details-Bin", "Content-Type"
// and "Accept" headers were already hidden in the previous version of
// the protocol.
//
// Also note that such commonly present headers as "Host" and "User-Agent" are
// still exposed as metadata, since they are already used in the wild and
// the protocol does not depend on them significantly.
//
// # v1.3
//
// v1.3 adds request/response body compression support using GZIP (RFC-1952).
//
//   - A request MAY have a header "Content-Encoding: gzip".
//     If it is present, then the server MUST decompress the request body before
//     unmarshaling the request message.
//   - A request MAY have a header "Accept-Encoding: gzip".
//     If the header is present, then the server response body MAY be compressed;
//     and the server SHOULD decide based on the response message size.
//     If the response body is compressed, then the response MUST include a
//     header "Content-Encoding: gzip".
//
// See also https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Encoding
// and https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Accept-Encoding
//
// # v1.2
//
// v1.2 is small, backward-compatible amendment to v1.1 that adds support for
// error details.
//
// Response header "X-Prpc-Status-Details-Bin" contains elements of
// google.rpc.Status.details field, one value per element, in the same order.
// The header value is a standard base64 string with padding of the encoded
// google.protobuf.Any, where the message encoding is the same as the response
// message encoding, i.e. depends on Accept request header.
//
// # v1.1
//
// v1.1 is small, backward-compatible amendment to v1.0 to address a
// security issue. Since it is backward compatible, it does not introduce
// a formal protocol version header at this time.
//
// Changes:
//   - Requests/responses must use "application/json" media type instead of
//     "application/prpc; encoding=json".
//   - Responses must include "X-Content-Type-Options: nosniff" HTTP header.
//
// This enables CORB protection (which mitigates spectre) and disables content
// sniffing.
// For CORB, see https://chromium.googlesource.com/chromium/src/+/master/services/network/cross_origin_read_blocking_explainer.md.
//
// # v1.0
//
// This section describes the base pRPC protocol. It is based on HTTP 1.x and
// employs gRPC codes.
//
// A pRPC server MUST handle HTTP POST requests at `/prpc/{service}/{method}`
// where service is a full service name including full package name.
// The handler must decode an input message from an HTTP request,
// call the service method implementation and
// encode the returned output message or error to the HTTP response.
//
// pRPC protocol defines three protocol buffer encodings and media types.
//   - Binary: "application/prpc; encoding=binary" (default).
//   - JSON:   "application/prpc; encoding=json"   or "application/json"
//     A response body MUST have `)]}'\n` prefix to prevent XSSI.
//   - Text:   "application/prpc; encoding=text"
//
// A pRPC server MUST support Binary and SHOULD support JSON and Text.
//
// Request headers:
//   - "X-Prpc-Grpc-Timeout": specifies request timeout.
//     A client MAY specify it.
//     If a service hits the timeout, a server MUST respond with HTTP 503 and
//     DeadlineExceed gRPC code.
//     Value format: `\d+[HMSmun]` (regular expression), where
//   - H - hour
//   - M - minute
//   - S - second
//   - m - millisecond
//   - u - microsecond
//   - n - nanosecond
//   - "Content-Type": specifies input message encoding in the body.
//     A client SHOULD specify it.
//     If not present, a server MUST treat the input message as Binary.
//   - "Accept": specifies the output message encoding for the response.
//     A client MAY specify it, a server MUST support it.
//   - Any other headers MUST be added to metadata.MD in the context that is
//     passed to the service method implementation (note this behavior was
//     amended in v1.4).
//   - If a header name has "-Bin" suffix, the server must treat it as
//     standard-base64-encoded with padding and put the decoded binary blob into
//     the metadata under the original key (i.e. the one ending with "-bin").
//
// Response headers:
//   - "X-Prpc-Grpc-Code": specifies the gRPC code.
//     A server MUST specify it in all cases.
//   - If it is present, the client MUST ignore the HTTP status code.
//   - If OK, the client MUST return the output message
//     decoded from the body.
//   - Otherwise, the client MUST return a gRPC error with the
//     code and message read from the response body.
//   - If not present, the client MUST return a non-gRPC error
//     with message read from the response body.
//     A client MAY read a portion of the response body.
//   - "Content-Type": specifies the output message encoding.
//     A server SHOULD specify it.
//     If not specified, a client MUST treat it is as Binary.
//   - Any metadata returned by a service method implementation MUST go into
//     http headers, unless metadata key starts with "X-Prpc-".
//
// A server MUST always specify "X-Prpc-Grpc-Code".
// The server SHOULD specify HTTP status corresponding to the gRPC code.
//
// If the "X-Prpc-Grpc-Code" response header value is not 0, the response body
// MUST contain a description of the error.
//
// If a service/method is not found, the server MUST respond with Unimplemented
// gRPC code and SHOULD specify HTTP 501 status.
package prpc
