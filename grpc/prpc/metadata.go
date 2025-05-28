// Copyright 2019 The LUCI Authors.
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
	"encoding/base64"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
)

// Standard HTTP headers that are vital for the pRPC protocol and should not
// be messed with via prpc.SetHeader(...).
var vitalHTTPHeaders = map[string]struct{}{
	"Accept":                 {}, // content type negotiation
	"Accept-Encoding":        {}, // content type negotiation
	"Content-Encoding":       {}, // request/response compression
	"Content-Length":         {}, // vital for HTTP
	"Content-Type":           {}, // based on negotiated content type
	"Vary":                   {}, // part of CORS response
	"X-Content-Type-Options": {}, // always set to "nosniff"
}

// isReservedMetadataKey returns true for disallowed metadata keys.
//
// Keys are given in HTTP header canonical format.
//
// Setting metadata with such keys may break the protocol.
// See also Client.prepareRequest.
func isReservedMetadataKey(k string) bool {
	// These standard HTTP headers are vital for the pRPC protocol and should not
	// be messed with.
	if _, yes := vitalHTTPHeaders[k]; yes {
		return true
	}

	// These headers are used internally by pRPC protocol and should not be
	// messed with.
	if strings.HasPrefix(k, "X-Prpc-") {
		return true
	}

	// CORS policies should be set via server.AccessControl callback, not via
	// individual RPCs.
	if strings.HasPrefix(k, "Access-Control-") {
		return true
	}

	return false
}

// metaIntoHeaders merges outgoing metadata into the given set of headers.
//
// Encodes metadata entries with keys that end with "-bin".
func metaIntoHeaders(md metadata.MD, h http.Header) error {
	for k, vs := range md {
		canon := http.CanonicalHeaderKey(k)
		if isReservedMetadataKey(canon) {
			return errors.Fmt("using reserved metadata key %q", k)
		}
		if !strings.HasSuffix(canon, "-Bin") {
			h[canon] = append(h[canon], vs...)
		} else {
			for _, v := range vs {
				h[canon] = append(h[canon], base64.StdEncoding.EncodeToString([]byte(v)))
			}
		}
	}
	return nil
}

// headerIntoMeta merges values of the given header key into the metadata.
//
// Decodes entries with keys that end with "-bin". See also readStatusDetails
// which also does this decoding specifically for "X-Prpc-Status-Details-Bin".
func headerIntoMeta(key string, values []string, md metadata.MD) error {
	key = strings.ToLower(key)
	if !strings.HasSuffix(key, "-bin") {
		md[key] = append(md[key], values...)
		return nil
	}
	for _, v := range values {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return err
		}
		md[key] = append(md[key], string(decoded))
	}
	return nil
}

// headersIntoMetadata returns a new metadata.MD constructed from given headers.
//
// All reserved headers are silently skipped. If there's nothing left, returns
// nil.
func headersIntoMetadata(h http.Header) (md metadata.MD, err error) {
	for k, v := range h {
		if isReservedMetadataKey(http.CanonicalHeaderKey(k)) {
			continue
		}
		if md == nil {
			md = make(metadata.MD, len(h))
		}
		if err := headerIntoMeta(k, v, md); err != nil {
			return nil, errors.Fmt("can't decode header %q: %w", k, err)
		}
	}
	return
}
