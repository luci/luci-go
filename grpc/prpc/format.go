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
	"fmt"
	"mime"
)

const (
	// ContentTypePRPC is the formal MIME content type for pRPC messages.
	ContentTypePRPC = "application/prpc"
	// ContentTypeJSON is the JSON content type.
	ContentTypeJSON = "application/json"

	mtPRPCEncoding       = "encoding"
	mtPRPCEncodingBinary = "binary"
	mtPRPCEncodingJSONPB = "json"
	mtPRPCEncodingText   = "text"
	mtPRPCBinary         = ContentTypePRPC + "; " + mtPRPCEncoding + "=" + mtPRPCEncodingBinary
	mtPRPCJSONPB         = ContentTypeJSON
	mtPRPCJSONPBLegacy   = ContentTypePRPC + "; " + mtPRPCEncoding + "=" + mtPRPCEncodingJSONPB
	mtPRPCText           = ContentTypePRPC + "; " + mtPRPCEncoding + "=" + mtPRPCEncodingText

	// JSONPBPrefix is prepended to a message in JSONPB format to avoid CSRF.
	JSONPBPrefix = ")]}'\n"
)

var bytesJSONPBPrefix = []byte(JSONPBPrefix)

// Format is the pRPC protobuf wire format specification.
type Format int

// (Ordered by preference).
const (
	// FormatBinary indicates that a message is encoded as a raw binary protobuf.
	FormatBinary Format = iota
	// FormatJSONPB indicates that a message is encoded as a JSON-serialized
	// protobuf.
	FormatJSONPB
	// FormatText indicates that a message is encoded as a text protobuf.
	FormatText
)

// FormatFromContentType converts Content-Type header value from a request to a
// format.
// Can return only FormatBinary, FormatJSONPB or FormatText.
// In case of an error, format is undefined.
func FormatFromContentType(v string) (Format, error) {
	if v == "" {
		return FormatBinary, nil
	}
	mediaType, mediaTypeParams, err := mime.ParseMediaType(v)
	if err != nil {
		return 0, err
	}
	return FormatFromMediaType(mediaType, mediaTypeParams)
}

// FormatFromMediaType converts a media type ContentType and its parameters
// into a pRPC Format.
// Can return only FormatBinary, FormatJSONPB or FormatText.
// In case of an error, format is undefined.
func FormatFromMediaType(mt string, params map[string]string) (Format, error) {
	switch mt {
	case ContentTypePRPC:
		for k := range params {
			if k != mtPRPCEncoding {
				return 0, fmt.Errorf("unexpected parameter %q", k)
			}
		}
		return FormatFromEncoding(params[mtPRPCEncoding])

	case ContentTypeJSON:
		return FormatJSONPB, nil

	case "", "*/*", "application/*", "application/grpc":
		return FormatBinary, nil

	default:
		return 0, fmt.Errorf("unknown content type: %q", mt)
	}
}

// FormatFromEncoding converts a media type encoding parameter into a pRPC
// Format.
// Can return only FormatBinary, FormatJSONPB or FormatText.
// In case of an error, format is undefined.
func FormatFromEncoding(v string) (Format, error) {
	switch v {
	case "", mtPRPCEncodingBinary:
		return FormatBinary, nil
	case mtPRPCEncodingJSONPB:
		return FormatJSONPB, nil
	case mtPRPCEncodingText:
		return FormatText, nil
	default:
		return 0, fmt.Errorf(`invalid encoding parameter: %q. Valid values: `+
			`"`+mtPRPCEncodingBinary+`", `+
			`"`+mtPRPCEncodingJSONPB+`", `+
			`"`+mtPRPCEncodingText+`"`, v)
	}
}

// MediaType returns a full media type for f.
func (f Format) MediaType() string {
	switch f {
	case FormatJSONPB:
		return mtPRPCJSONPB
	case FormatText:
		return mtPRPCText
	case FormatBinary:
		fallthrough
	default:
		return mtPRPCBinary
	}
}
