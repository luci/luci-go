// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

// This file implements encoding of RPC results to HTTP responses.

import (
	"bytes"
	"net/http"
	"sort"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const (
	headerAccept = "Accept"
	csrfPrefix   = ")]}'\n"
)

// responseFormat returns the format to be used in a response.
// Can return only formatBinary (preferred), formatJSONPB or formatText.
// In case of an error, format is undefined.
func responseFormat(acceptHeader string) (format, *protocolError) {
	if acceptHeader == "" {
		return formatBinary, nil
	}

	parsed, err := parseAccept(acceptHeader)
	if err != nil {
		return formatBinary, errorf(http.StatusBadRequest, "Accept header: %s", err)
	}
	formats := make(acceptFormatSlice, 0, len(parsed))
	for _, at := range parsed {
		f, err := parseFormat(at.MediaType, at.MediaTypeParams)
		if err != nil {
			// Ignore invalid format. Check further.
			continue
		}
		switch f {

		case formatBinary, formatJSONPB, formatText:
			// fine

		case formatUnspecified:
			f = formatBinary // prefer binary

		default:
			continue
		}

		formats = append(formats, acceptFormat{f, at.QualityFactor})
	}
	if len(formats) == 0 {
		return formatBinary, errorf(
			http.StatusNotAcceptable,
			"Accept header: specified media types are not not supported. Supported types: %q, %q, %q, %q.",
			mtPRPCBinary,
			mtPRPCJSNOPB,
			mtPRPCText,
			mtJSON,
		)
	}
	sort.Sort(formats) // order by quality factor and format preference.
	return formats[0].Format, nil
}

// respondMessage encodes msg to a response in the specified format.
func respondMessage(msg proto.Message, format format) *response {
	if msg == nil {
		return errResponse(codes.Internal, 0, "pRPC: responseMessage: msg is nil")
	}
	res := response{header: http.Header{}}
	var err error
	switch format {
	case formatBinary:
		res.header.Set(headerContentType, mtPRPCBinary)
		res.body, err = proto.Marshal(msg)

	case formatJSONPB:
		res.header.Set(headerContentType, mtPRPCJSNOPB)
		var buf bytes.Buffer
		buf.WriteString(csrfPrefix)
		m := jsonpb.Marshaler{}
		err = m.Marshal(&buf, msg)
		if err == nil {
			_, err = buf.WriteRune('\n')
		}
		res.body = buf.Bytes()

	case formatText:
		res.header.Set(headerContentType, mtPRPCText)
		var buf bytes.Buffer
		err = proto.MarshalText(&buf, msg)
		res.body = buf.Bytes()

	default:
		return errResponse(codes.Internal, 0, "pRPC: responseMessage: invalid format %s", format)

	}
	if err != nil {
		return errResponse(codes.Internal, 0, escapeFmt(err.Error()))
	}

	return &res
}

// respondProtocolError creates a response for a pRPC protocol error.
func respondProtocolError(err *protocolError) *response {
	return errResponse(codes.InvalidArgument, err.status, escapeFmt(err.err.Error()))
}

// errorCode returns a most appropriate gRPC code for an error
func errorCode(err error) codes.Code {
	switch err {
	case context.DeadlineExceeded:
		return codes.DeadlineExceeded

	case context.Canceled:
		return codes.Canceled

	default:
		return grpc.Code(err)
	}
}

// codeToStatus maps gRPC codes to HTTP statuses.
// This map may need to be corrected when
// https://github.com/grpc/grpc-common/issues/210
// is closed.
var codeToStatus = map[codes.Code]int{
	codes.OK:                 http.StatusOK,
	codes.Canceled:           http.StatusNoContent,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DeadlineExceeded:   http.StatusServiceUnavailable,
	codes.NotFound:           http.StatusNotFound,
	codes.AlreadyExists:      http.StatusConflict,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.Unauthenticated:    http.StatusUnauthorized,
	codes.ResourceExhausted:  http.StatusServiceUnavailable,
	codes.FailedPrecondition: http.StatusPreconditionFailed,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.Unimplemented:      http.StatusNotImplemented,
	codes.Unavailable:        http.StatusServiceUnavailable,
}

// codeStatus maps gRPC codes to HTTP status codes.
// Falls back to http.StatusInternalServerError.
func codeStatus(code codes.Code) int {
	if status, ok := codeToStatus[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}
