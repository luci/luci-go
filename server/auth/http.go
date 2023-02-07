// Copyright 2023 The LUCI Authors.
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

package auth

import (
	"net/http"
)

type httpRequestMetadata struct {
	r *http.Request
}

func (r httpRequestMetadata) Header(key string) string                { return r.r.Header.Get(key) }
func (r httpRequestMetadata) Cookie(key string) (*http.Cookie, error) { return r.r.Cookie(key) }
func (r httpRequestMetadata) RemoteAddr() string                      { return r.r.RemoteAddr }
func (r httpRequestMetadata) Host() string                            { return r.r.Host }

// RequestMetadataForHTTP returns a RequestMetadata of a given http.Request.
func RequestMetadataForHTTP(r *http.Request) RequestMetadata {
	return httpRequestMetadata{r}
}
