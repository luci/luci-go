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
	"io/ioutil"
	"net/http"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	luciproto "go.chromium.org/luci/common/proto"
)

// This file implements decoding of HTTP requests to RPC parameters.

const headerContentType = "Content-Type"

// readMessage decodes a protobuf message from an HTTP request.
// Does not close the request body.
// fixFieldMasksForJSON indicates whether to attempt a workaround for
// https://github.com/golang/protobuf/issues/745 for requests with FormatJSONPB.
// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
func readMessage(r *http.Request, msg proto.Message, fixFieldMasksForJSON bool) *protocolError {
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
		return errorf(http.StatusBadRequest, "could not decode body: %s", err)
	}
	return nil
}

// parseHeader parses HTTP headers and derives a new context.
// Supports HeaderTimeout.
// Ignores "Accept" and "Content-Type" headers.
// If host is not empty, adds "host" metadata.
//
// If there are unrecognized HTTP headers, with or without headerSuffixBinary,
// they are added to a metadata.MD and a new context is derived.
// If ctx already has metadata, the latter is copied.
//
// In case of an error, returns ctx unmodified.
func parseHeader(ctx context.Context, header http.Header, host string) (context.Context, error) {
	origC := ctx

	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}

	addedMeta := false
	for name, values := range header {
		if len(values) == 0 {
			continue
		}
		name = http.CanonicalHeaderKey(name)
		switch name {

		case HeaderTimeout:
			// Decode only first value, ignore the rest
			// to be consistent with http.Header.Get.
			timeout, err := DecodeTimeout(values[0])
			if err != nil {
				return origC, fmt.Errorf("%s header: %s", HeaderTimeout, err)
			}
			// TODO(crbug/1006920): Do not leak the cancel context.
			ctx, _ = clock.WithTimeout(ctx, timeout)

		case headerAccept, headerContentType:
		// readMessage and writeMessage handle these headers.

		default:
			addedMeta = true
			for _, v := range values {
				mdKey, mdValue, err := headerToMeta(name, v)
				if err != nil {
					return origC, fmt.Errorf("%s header: %s", name, err)
				}
				md.Append(mdKey, mdValue)
			}
		}
	}

	if host != "" {
		md.Append("host", host)
		addedMeta = true
	}

	if addedMeta {
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	return ctx, nil
}
