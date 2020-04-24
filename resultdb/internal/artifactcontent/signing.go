// Copyright 2020 The LUCI Authors.
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

package artifactcontent

import (
	"context"
	"net/url"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/tokens"
)

var artifactNameTokenKind = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: time.Hour,
	SecretKey:  "artifact_name",
	Version:    1,
}

// GenerateSignedURL generates a signed HTTPS URL back to this server.
// The returned token works only with the same artifact name.
func (s *Server) GenerateSignedURL(ctx context.Context, artifactName string) (u *url.URL, expiration time.Time, err error) {
	const ttl = time.Hour
	now := clock.Now(ctx).UTC()

	tok, err := artifactNameTokenKind.Generate(ctx, []byte(artifactName), nil, ttl)
	if err != nil {
		return nil, time.Time{}, err
	}

	q := url.Values{}
	q.Set("token", tok)
	u = &url.URL{
		Scheme:   "https",
		Host:     s.Hostname,
		Path:     "/" + artifactName,
		RawQuery: q.Encode(),
	}
	if s.InsecureURLs {
		u.Scheme = "http"
	}
	expiration = now.Add(ttl)
	return
}
