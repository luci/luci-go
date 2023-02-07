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

package authtest

import (
	"net/http"
)

// FakeRequestMetadata implements RequestMetadata using fake values.
type FakeRequestMetadata struct {
	FakeHeader     http.Header
	FakeCookies    map[string]*http.Cookie
	FakeRemoteAddr string
	FakeHost       string
}

// NewFakeRequestMetadata constructs an empty FakeRequestMetadata.
func NewFakeRequestMetadata() *FakeRequestMetadata {
	return &FakeRequestMetadata{
		FakeHeader:     map[string][]string{},
		FakeCookies:    map[string]*http.Cookie{},
		FakeRemoteAddr: "127.0.0.1",
		FakeHost:       "fake.example.com",
	}
}

// Header is part of RequestMetadata interface.
func (r *FakeRequestMetadata) Header(key string) string {
	return r.FakeHeader.Get(key)
}

// Cookie is part of RequestMetadata interface.
func (r *FakeRequestMetadata) Cookie(key string) (*http.Cookie, error) {
	if c, ok := r.FakeCookies[key]; ok {
		return c, nil
	}
	return nil, http.ErrNoCookie
}

// RemoteAddr is part of RequestMetadata interface.
func (r *FakeRequestMetadata) RemoteAddr() string {
	return r.FakeRemoteAddr
}

// Host is part of RequestMetadata interface.
func (r *FakeRequestMetadata) Host() string {
	return r.FakeHost
}
