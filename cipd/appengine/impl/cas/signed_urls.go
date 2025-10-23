// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

const (
	minSignedURLExpiration = 30 * time.Minute
	maxSignedURLExpiration = 2 * time.Hour
	absenceExpiration      = time.Minute
)

// signedURLParams describe what signed URL we want to get.
//
// They are also used to construct a key for the signed URL cache.
type signedURLParams struct {
	// GsPath is a "/bucket/path" identifying a GCS object.
	GsPath string
	// Filename to place into Content-Disposition header when fetching the URL.
	Filename string
	// UserProject is a GCP project to bill download bandwidth to.
	UserProject string
}

// cacheKey is the key representing the cache entry matching signedURLParams.
//
// Its value doesn't leave CIPD backend and its format doesn't matter much
// (used only as a map lookup key).
func (s *signedURLParams) cacheKey() string {
	return url.Values{
		"path":        {s.GsPath},
		"filename":    {s.Filename},
		"userproject": {s.UserProject},
	}.Encode()
}

// gsObjInfo is stored in the signed URL cache.
type gsObjInfo struct {
	// Size is the GCS object size in bytes.
	Size uint64 `json:"size,omitempty"`
	// URL is the signed URL that can be used to fetch the object.
	URL string `json:"url,omitempty"`
}

// Exists returns whether this info refers to a file which exists.
func (i *gsObjInfo) Exists() bool {
	if i == nil {
		return false
	}
	return i.URL != ""
}

// signedURLParams.cacheKey() => gsObjInfo{...}.
var signedURLsCache = layered.RegisterCache(layered.Parameters[*gsObjInfo]{
	ProcessCacheCapacity: 65536,
	GlobalNamespace:      "signed_gs_urls_v3",
	Marshal: func(item *gsObjInfo) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (*gsObjInfo, error) {
		out := &gsObjInfo{}
		err := json.Unmarshal(blob, out)
		return out, err
	},
})

// getSignedURL returns a signed URL that can be used to fetch the given file
// as well as the size of that file in bytes.
//
// 'params.GsPath' should have form '/bucket/path' or the call will panic.
// 'params.Filename', if given, will be returned in Content-Disposition header
// when accessing the signed URL. It instructs user agents to save the file
// under the given name.
//
// The returned URL is valid for at least 30 min (may be longer). It's expected
// that it will be used right away, not stored somewhere.
//
// On failures returns grpc-annotated errors. In particular, if the requested
// file is missing, returns NotFound grpc-annotated error.
func getSignedURL(ctx context.Context, signer signerFactory, gsstore gs.GoogleStorage, params *signedURLParams) (string, uint64, error) {
	info, err := signedURLsCache.GetOrCreate(ctx, params.cacheKey(), func() (*gsObjInfo, time.Duration, error) {
		info := &gsObjInfo{}
		switch size, yes, err := gsstore.Size(ctx, params.GsPath); {
		case err != nil:
			return nil, 0, errors.Fmt("failed to check GS file presence: %w", err)
		case !yes:
			return info, absenceExpiration, nil
		default:
			info.Size = size
		}

		// Query parameters to include into the final URL (they are signed as well).
		queryParams := url.Values{}
		if params.Filename != "" {
			if strings.ContainsAny(params.Filename, "\"\r\n") {
				panic("bad filename for Content-Disposition header")
			}
			queryParams.Set("response-content-disposition", fmt.Sprintf(`attachment; filename="%s"`, params.Filename))
		}
		if params.UserProject != "" {
			queryParams.Set("userProject", params.UserProject)
		}

		// An implementation of SignBytes.
		sig, err := signer(ctx)
		if err != nil {
			return nil, 0, errors.Fmt("can't create the signer: %w", err)
		}

		bucket, object := gs.SplitPath(params.GsPath)
		url, err := storage.SignedURL(bucket, object, &storage.SignedURLOptions{
			Scheme:         storage.SigningSchemeV4,
			GoogleAccessID: sig.Email,
			SignBytes: func(blob []byte) ([]byte, error) {
				_, signature, err := sig.SignBytes(ctx, blob)
				return signature, err
			},
			Method:          "GET",
			QueryParameters: queryParams,
			// Note storage.SignedURL(...) uses time.Now() internally, we can't mock
			// the time here. Tests will have to work with the real time.
			Expires: time.Now().Add(maxSignedURLExpiration),
		})
		if err != nil {
			return nil, 0, err
		}

		// 'url' here is valid for maxSignedURLExpiration. By caching it for
		// 'max-min' seconds, right before the cache expires the URL will have
		// lifetime of max-(max-min) == min, which is what we want.
		info.URL = url
		return info, maxSignedURLExpiration - minSignedURLExpiration, nil
	})

	if err != nil {
		return "", 0, grpcutil.InternalTag.Apply(errors.Fmt("failed to sign URL: %w", err))
	}

	if !info.Exists() {
		return "", 0, grpcutil.NotFoundTag.Apply(errors.Fmt("object %q doesn't exist", params.GsPath))
	}

	return info.URL, info.Size, nil
}
