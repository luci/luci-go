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
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"

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
			FormatBinary.MediaType(),
			FormatJSONPB.MediaType(),
			FormatText.MediaType(),
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
	w.Header().Set(headerContentType, format.MediaType())
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(body); err != nil {
		logging.WithError(err).Errorf(c, "prpc: failed to write response body")
	}
}

func errorStatus(err error) (st *status.Status, httpStatus int) {
	if err == nil {
		panic("err is nil")
	}

	err = errors.Unwrap(err)
	st, isStatus := status.FromError(err) // initializes st unconditionally.
	perr, isPerr := err.(*protocolError)
	switch {
	case isStatus:
	// great.

	case isPerr:
		st = status.New(codes.InvalidArgument, perr.err.Error())
		httpStatus = perr.status

	case err == context.Canceled:
		st = status.New(codes.Canceled, "canceled")

	case err == context.DeadlineExceeded:
		st = status.New(codes.DeadlineExceeded, "deadline exceeded")

	default:
		// st is non-nil because err is non-nil.
	}

	if httpStatus == 0 {
		httpStatus = grpcutil.CodeStatus(st.Code())
	}
	return
}

// writeStatusDetails returns response headers for status details.
func statusDetailsToHeaders(details []*any.Any) ([]string, error) {
	ret := make([]string, len(details))

	var buf proto.Buffer
	for i, det := range details {
		buf.Reset()
		if err := buf.EncodeMessage(det); err != nil {
			return nil, err
		}
		msgBytes := buf.Bytes()[:proto.Size(det)]
		ret[i] = base64.StdEncoding.EncodeToString(msgBytes)
	}

	return ret, nil
}

// writeError writes err to w and logs it.
func writeError(c context.Context, w http.ResponseWriter, err error) {
	st, httpStatus := errorStatus(err)

	// use st.Proto instead of st.Details to avoid unnecessary unmarshaling of
	// google.protobuf.Any sub-messages.
	detailHeader, err := statusDetailsToHeaders(st.Proto().Details)
	if err != nil {
		st = status.New(codes.Internal, "prpc: failed to write status details")
		httpStatus = http.StatusInternalServerError
	} else {
		w.Header()[HeaderStatusDetail] = detailHeader
	}

	body := st.Message()
	level := logging.Warning
	if httpStatus >= 500 {
		level = logging.Error
		// Hide potential implementation details from the user.
		body = http.StatusText(httpStatus)
	}
	logging.Logf(c, level, "prpc: responding with %s error: %s", st.Code(), st.Message())

	w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(st.Code())))
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
