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

package prpc

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// This file implements decoding of HTTP requests to RPC parameters.

const headerContentType = "Content-Type"

// readMessage decodes a protobuf message from an HTTP request.
// Does not close the request body.
func readMessage(r *http.Request, msg proto.Message) *protocolError {
	format, err := FormatFromContentType(r.Header.Get(headerContentType))
	if err != nil {
		// Spec: http://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.16
		return errorf(http.StatusUnsupportedMediaType, "Content-Type header: %s", err)
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errorf(http.StatusBadRequest, "could not read body: %s", err)
	}
	switch format {
	// Do not redefine "err" below.

	case FormatJSONPB:
		err = protojson.Unmarshal(buf, msg)
	case FormatText:
		err = prototext.Unmarshal(buf, msg)

	case FormatBinary:
		err = proto.Unmarshal(buf, msg)

	default:
		panic(fmt.Errorf("impossible: invalid format %v", format))
	}
	if err != nil {
		return errorf(http.StatusBadRequest, "could not decode body: %s", err)
	}
	return nil
}

// parseHeader parses HTTP headers and derives a new context.
//
// Recognizes "X-Prpc-Grpc-Timeout" header. Other recognized reserved headers
// are skipped. If there are unrecognized HTTP headers they are added to
// a metadata.MD and a new context is derived. If ctx already has metadata,
// the latter is copied.
//
// If host is not empty, sets "host" metadata.
func parseHeader(ctx context.Context, header http.Header, host string) (context.Context, context.CancelFunc, error) {
	// Parse headers into a metadata map. This skips all reserved headers to avoid
	// leaking pRPC protocol implementation details to gRPC servers.
	parsed, err := headersIntoMetadata(header)
	if err != nil {
		return nil, nil, err
	}

	// Modify metadata.MD in the context only if we have something to add.
	if parsed != nil || host != "" {
		existing, _ := metadata.FromIncomingContext(ctx)
		merged := metadata.Join(existing, parsed)
		if host != "" {
			merged["host"] = []string{host}
		}
		ctx = metadata.NewIncomingContext(ctx, merged)
	}

	// Parse the reserved timeout header, if any. Apply to the context.
	if timeoutStr := header.Get(HeaderTimeout); timeoutStr != "" {
		timeout, err := DecodeTimeout(timeoutStr)
		if err != nil {
			return nil, nil, errors.Annotate(err, "%q header", HeaderTimeout).Err()
		}
		ctx, cancel := clock.WithTimeout(ctx, timeout)
		return ctx, cancel, nil
	}
	return ctx, func() {}, nil
}
