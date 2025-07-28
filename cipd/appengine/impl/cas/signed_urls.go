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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
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

type gsObjInfo struct {
	Size uint64 `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Exists returns whether this info refers to a file which exists.
func (i *gsObjInfo) Exists() bool {
	if i == nil {
		return false
	}
	return i.URL != ""
}

// GS path (string) => details about the file at that path (gsObjInfo).
var signedURLsCache = layered.RegisterCache(layered.Parameters[*gsObjInfo]{
	ProcessCacheCapacity: 4096,
	GlobalNamespace:      "signed_gs_urls_v2",
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
// 'gsPath' should have form '/bucket/path' or the call will panic. 'filename',
// if given, will be returned in Content-Disposition header when accessing the
// signed URL. It instructs user agents to save the file under the given name.
//
// 'signAs' is an email of a service account to impersonate when signing or ""
// to use the default service account.
//
// The returned URL is valid for at least 30 min (may be longer). It's expected
// that it will be used right away, not stored somewhere.
//
// On failures returns grpc-annotated errors. In particular, if the requested
// file is missing, returns NotFound grpc-annotated error.
func getSignedURL(ctx context.Context, gsPath, filename string, signer signerFactory, gs gs.GoogleStorage) (string, uint64, error) {
	info, err := signedURLsCache.GetOrCreate(ctx, gsPath, func() (*gsObjInfo, time.Duration, error) {
		info := &gsObjInfo{}
		switch size, yes, err := gs.Size(ctx, gsPath); {
		case err != nil:
			return nil, 0, errors.Fmt("failed to check GS file presence: %w", err)
		case !yes:
			return info, absenceExpiration, nil
		default:
			info.Size = size
		}

		sig, err := signer(ctx)
		if err != nil {
			return nil, 0, errors.Fmt("can't create the signer: %w", err)
		}

		url, err := signURL(ctx, gsPath, sig, maxSignedURLExpiration)
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
		return "", 0,
			grpcutil.InternalTag.Apply(errors.Fmt("failed to sign URL: %w", err))
	}

	if !info.Exists() {
		return "", 0,
			grpcutil.NotFoundTag.Apply(errors.Fmt("object %q doesn't exist", gsPath))
	}

	signedURL := info.URL
	// Oddly, response-content-disposition is not signed and can be slapped onto
	// existing signed URL. We don't complain though, makes live easier.
	if filename != "" {
		if strings.ContainsAny(filename, "\"\r\n") {
			panic("bad filename for Content-Disposition header")
		}
		v := url.Values{
			"response-content-disposition": {
				fmt.Sprintf(`attachment; filename="%s"`, filename),
			},
		}
		signedURL += "&" + v.Encode()
	}

	return signedURL, info.Size, nil
}

// signURL generates a signed GS URL using the signer.
func signURL(ctx context.Context, gsPath string, signer *signer, expiry time.Duration) (string, error) {
	// See https://cloud.google.com/storage/docs/access-control/signed-urls.
	//
	// Basically, we sign a specially crafted multi-line string that encodes
	// expected parameters of the request. During the actual request, Google
	// Storage backend will construct the same string and verify that the provided
	// signature matches it.

	expires := fmt.Sprintf("%d", clock.Now(ctx).Add(expiry).Unix())

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "GET\n")
	fmt.Fprintf(buf, "\n") // expected value of 'Content-MD5' header, not used
	fmt.Fprintf(buf, "\n") // expected value of 'Content-Type' header, not used
	fmt.Fprintf(buf, "%s\n", expires)
	fmt.Fprintf(buf, "%s", gsPath)

	_, sig, err := signer.SignBytes(ctx, buf.Bytes())
	if err != nil {
		return "", errors.Fmt("signBytes call failed: %w", err)
	}

	u := url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   gsPath,
		RawQuery: (url.Values{
			"GoogleAccessId": {signer.Email},
			"Expires":        {expires},
			"Signature":      {base64.StdEncoding.EncodeToString(sig)},
		}).Encode(),
	}
	return u.String(), nil
}
