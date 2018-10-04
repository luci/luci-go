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

// This file implements encoding of RPC results to HTTP responses.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
)

const (
	headerAccept = "Accept"
)

// responseFormat returns the format to be used in a response.
// Can return only FormatBinary (preferred), FormatJSONPB or FormatText.
// In case of an error, format is undefined.
func responseFormat(acceptHeader string) (Format, *protocolError) {
	if acceptHeader == "" {
		return FormatBinary, nil
	}

	parsed, err := parseAccept(acceptHeader)
	if err != nil {
		return FormatBinary, errorf(http.StatusBadRequest, "Accept header: %s", err)
	}
	formats := make(acceptFormatSlice, 0, len(parsed))
	for _, at := range parsed {
		f, err := FormatFromMediaType(at.MediaType, at.MediaTypeParams)
		if err != nil {
			// Ignore invalid format. Check further.
			continue
		}
		formats = append(formats, acceptFormat{f, at.QualityFactor})
	}
	if len(formats) == 0 {
		return FormatBinary, errorf(
			http.StatusNotAcceptable,
			"Accept header: specified media types are not not supported. Supported types: %q, %q, %q, %q.",
			FormatBinary.ContentType(),
			FormatJSONPB.ContentType(),
			FormatText.ContentType(),
			ContentTypeJSON,
		)
	}
	sort.Sort(formats) // order by quality factor and format preference.
	return formats[0].Format, nil
}

// writeMessage writes msg to w in the specified format.
// c is used to log errors.
// panics if msg is nil.
func writeMessage(c context.Context, w http.ResponseWriter, msg proto.Message, format Format) {
	if msg == nil {
		panic("msg is nil")
	}

	var body []byte
	var err error
	switch format {
	case FormatBinary:
		body, err = proto.Marshal(msg)

	case FormatJSONPB:
		var buf bytes.Buffer
		buf.WriteString(JSONPBPrefix)
		m := jsonpb.Marshaler{}
		err = m.Marshal(&buf, msg)
		if err == nil {
			_, err = buf.WriteRune('\n')
		}
		body = buf.Bytes()

	case FormatText:
		var buf bytes.Buffer
		err = proto.MarshalText(&buf, msg)
		body = buf.Bytes()

	default:
		panic(fmt.Errorf("impossible: invalid format %d", format))

	}
	if err != nil {
		writeError(c, w, withCode(err, codes.Internal))
		return
	}

	w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.OK)))
	w.Header().Set(headerContentType, format.ContentType())
	if _, err := w.Write(body); err != nil {
		logging.WithError(err).Errorf(c, "prpc: failed to write response body")
	}
}

// errorCode returns a most appropriate gRPC code for an error
func errorCode(err error) codes.Code {
	switch errors.Unwrap(err) {
	case context.DeadlineExceeded:
		return codes.DeadlineExceeded

	case context.Canceled:
		return codes.Canceled

	default:
		return grpc.Code(err)
	}
}

// writeError writes err to w and logs it.
func writeError(c context.Context, w http.ResponseWriter, err error) {
	var code codes.Code
	var httpStatus int
	var msg string
	if perr, ok := err.(*protocolError); ok {
		code = codes.InvalidArgument
		msg = perr.err.Error()
		httpStatus = perr.status
	} else {
		code = errorCode(err)
		msg = grpc.ErrorDesc(err)
		httpStatus = grpcutil.CodeStatus(code)
	}

	body := msg
	level := logging.Warning
	if httpStatus >= 500 {
		level = logging.Error
		// Hide potential implementation details from the user.
		body = http.StatusText(httpStatus)
	}
	logging.Logf(c, level, "prpc: responding with %s error: %s", code, msg)

	w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(code)))
	w.Header().Set(headerContentType, "text/plain")
	w.WriteHeader(httpStatus)
	if _, err := io.WriteString(w, body); err != nil {
		logging.WithError(err).Errorf(c, "prpc: failed to write response body")
		// The header is already written. There is nothing more we can do.
		return
	}
	io.WriteString(w, "\n")
}

func withCode(err error, c codes.Code) error {
	return status.Error(c, err.Error())
}
