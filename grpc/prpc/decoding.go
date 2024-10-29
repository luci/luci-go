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
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	luciproto "go.chromium.org/luci/common/proto"

	"go.chromium.org/luci/grpc/grpcutil"
)

// This file implements decoding of HTTP requests to RPC parameters.

const headerContentType = "Content-Type"

// decompressionLimitErr is returned if readMessage decompressed too much data.
var decompressionLimitErr = errors.Reason("the decompressed request size exceeds the server limit").Err()

// readMessage decodes a protobuf message from an HTTP request.
//
// Uses given headers to decide how to uncompress and deserialize the message.
// When decompressing, makes sure to decompress no more than
// `maxDecompressedBytes`. Assumes `body` is already a limited reader or
// it doesn't exceed a limit (see Server.maxBytesReader).
//
// fixFieldMasksForJSON indicates whether to attempt a workaround for
// https://github.com/golang/protobuf/issues/745 for requests with FormatJSONPB.
// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
func readMessage(body io.Reader, header http.Header, msg proto.Message, maxDecompressedBytes int64, fixFieldMasksForJSON bool) error {
	format, err := FormatFromContentType(header.Get(headerContentType))
	if err != nil {
		// Spec: http://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.16
		return protocolErr(
			codes.InvalidArgument,
			http.StatusUnsupportedMediaType,
			"bad Content-Type header: %s", err,
		)
	}

	var buf []byte
	if header.Get("Content-Encoding") == "gzip" {
		reader, err := getGZipReader(body)
		if err != nil {
			return requestReadErr(err, "decompressing the request")
		}
		if maxDecompressedBytes == math.MaxInt64 {
			// No limit. Just read until the EOF or OOM.
			buf, err = io.ReadAll(reader)
		} else {
			// Read one extra byte. This is how we'll know that we read past the
			// limit, because LimitReader uses io.EOF to indicate the limit is
			// reached, which is indistinguishable from just truncating the original
			// reader.
			buf, err = io.ReadAll(io.LimitReader(reader, maxDecompressedBytes+1))
			if err == nil {
				if int64(len(buf)) > maxDecompressedBytes {
					err = decompressionLimitErr
				}
			}
		}
		if err == nil {
			err = reader.Close() // this just checks the crc32 checksum
		} else {
			_ = reader.Close() // this will be some bogus error since we abandoned the reader
		}
		returnGZipReader(reader)
		if err != nil {
			return requestReadErr(err, "decompressing the request")
		}
	} else {
		buf, err = io.ReadAll(body)
		if err != nil {
			return requestReadErr(err, "reading the request")
		}
	}

	switch format {
	// Do not redefine "err" below.

	case FormatJSONPB:
		if fixFieldMasksForJSON {
			t := reflect.TypeOf(msg)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			buf, err = luciproto.FixFieldMasksBeforeUnmarshal(buf, t)
		}
		if err == nil {
			err = jsonpb.Unmarshal(bytes.NewBuffer(buf), msg)
		}

	case FormatText:
		err = proto.UnmarshalText(string(buf), msg)

	case FormatBinary:
		err = proto.Unmarshal(buf, msg)

	default:
		panic(fmt.Errorf("impossible: invalid format %v", format))
	}
	if err != nil {
		return protocolErr(
			codes.InvalidArgument,
			http.StatusBadRequest,
			"could not decode body: %s", err,
		)
	}
	return nil
}

// requestReadErr interprets an error from reading and unzipping of a request.
func requestReadErr(err error, msg string) *protocolError {
	var maxBytesErr *http.MaxBytesError

	code := codes.InvalidArgument
	errMsg := err.Error()
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.Canceled):
		code = codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case errors.Is(err, decompressionLimitErr):
		code = codes.Unavailable
	case errors.As(err, &maxBytesErr):
		code = codes.Unavailable
		errMsg = "the request size exceeds the server limit"
	}

	return protocolErr(code, grpcutil.CodeStatus(code), "%s: %s", msg, errMsg)
}

// parseHeader parses HTTP headers and derives a new context.
//
// Recognizes "X-Prpc-Grpc-Timeout" header. Other recognized reserved headers
// are skipped. If there are unrecognized HTTP headers they are added to
// a metadata.MD and a new context is derived. If ctx already has metadata,
// the latter is copied.
//
// If host is not empty, sets "host" and ":authority" metadata.
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
			merged[":authority"] = []string{host}
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
