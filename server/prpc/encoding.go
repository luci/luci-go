// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

// This file implements encoding of RPC results to HTTP responses.

import (
	"bytes"
	"io"
	"net/http"
	"sort"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/logging"
)

const (
	headerAccept = "Accept"
)

// responseFormat returns the format to be used in a response.
// Can return only formatBinary (preferred), formatJSONPB or formatText.
// In case of an error, format is undefined and the error has an HTTP status.
func responseFormat(acceptHeader string) (format, *httpError) {
	if acceptHeader == "" {
		return formatBinary, nil
	}

	parsed, err := parseAccept(acceptHeader)
	if err != nil {
		return formatBinary, errorf(http.StatusBadRequest, "Accept header: %s", err)
	}
	assert(len(parsed) > 0)
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

		case formatUnrecognized:
			continue

		default:
			panicf("cannot happen")
		}

		assert(f == formatBinary || f == formatJSONPB || f == formatText)
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

// writeMessage writes a protobuf message to response in the specified format.
func writeMessage(w http.ResponseWriter, msg proto.Message, format format) error {
	if msg == nil {
		panic("msg is nil")
	}
	var (
		contentType string
		res         []byte
		err         error
	)
	switch format {
	case formatBinary:
		contentType = mtPRPCBinary
		res, err = proto.Marshal(msg)

	case formatJSONPB:
		contentType = mtPRPCJSNOPB
		m := jsonpb.Marshaler{Indent: "\t"}
		var buf bytes.Buffer
		err = m.Marshal(&buf, msg)
		buf.WriteString("\n")
		res = buf.Bytes()

	case formatText:
		contentType = mtPRPCText
		var buf bytes.Buffer
		err = proto.MarshalText(&buf, msg)
		res = buf.Bytes()
	}
	if err != nil {
		return err
	}
	w.Header().Set(headerContentType, contentType)
	_, err = w.Write(res)
	return err
}

// codeToStatus maps gRPC codes to HTTP statuses.
// This map may need to be corrected when
// https://github.com/grpc/grpc-common/issues/210
// is closed.
var codeToStatus = map[codes.Code]int{
	codes.OK:                 http.StatusOK,
	codes.Canceled:           http.StatusNoContent,
	codes.Unknown:            http.StatusInternalServerError,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DeadlineExceeded:   http.StatusServiceUnavailable,
	codes.NotFound:           http.StatusNotFound,
	codes.AlreadyExists:      http.StatusConflict,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.Unauthenticated:    http.StatusUnauthorized,
	codes.ResourceExhausted:  http.StatusServiceUnavailable,
	codes.FailedPrecondition: http.StatusPreconditionFailed,
	codes.Aborted:            http.StatusInternalServerError,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.Unimplemented:      http.StatusNotImplemented,
	codes.Internal:           http.StatusInternalServerError,
	codes.Unavailable:        http.StatusServiceUnavailable,
	codes.DataLoss:           http.StatusInternalServerError,
}

// ErrorStatus returns HTTP status for an error.
// In particular, it maps gRPC codes to HTTP statuses.
// Status of nil is 200.
//
// See also grpc.Code.
func ErrorStatus(err error) int {
	if err, ok := err.(*httpError); ok {
		return err.status
	}

	status, ok := codeToStatus[grpc.Code(err)]
	if !ok {
		status = http.StatusInternalServerError
	}
	return status
}

// ErrorDesc returns the error description of err if it was produced by pRPC or gRPC.
// Otherwise, it returns err.Error() or empty string when err is nil.
//
// See also grpc.ErrorDesc.
func ErrorDesc(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*httpError); ok {
		err = e.err
	}
	return grpc.ErrorDesc(err)
}

// writeError writes an error to an HTTP response.
//
// HTTP status is determined by ErrorStatus.
// If it is http.StatusInternalServerError, prints only "Internal server error",
// otherwise uses ErrorDesc.
//
// Logs all errors with status >= 500.
func writeError(c context.Context, w http.ResponseWriter, err error) {
	if err == nil {
		panic("err is nil")
	}

	status := ErrorStatus(err)
	if status >= 500 {
		logging.Errorf(c, "HTTP %d: %s", status, ErrorDesc(err))
	}

	w.Header().Set(headerContentType, "text/plain")
	w.WriteHeader(status)

	var body string
	if status == http.StatusInternalServerError {
		body = "Internal server error"
	} else {
		body = ErrorDesc(err)
	}
	if _, err := io.WriteString(w, body+"\n"); err != nil {
		logging.Errorf(c, "could not write error: %s", err)
	}
}

func assert(condition bool) {
	if !condition {
		panicf("assertion failed")
	}
}
