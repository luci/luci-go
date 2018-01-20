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
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

const (
	minSignedURLExpiration = 30 * time.Minute
	maxSignedURLExpiration = 2 * time.Hour
)

// GS path (string) => signed URL (string) or "" if the file is missing.
var signedURLsCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(4096),
	GlobalNamespace: "signed_gs_urls_v1",
	Marshal:         func(item interface{}) ([]byte, error) { return []byte(item.(string)), nil },
	Unmarshal:       func(blob []byte) (interface{}, error) { return string(blob), nil },
}

// getSignedURL returns signed URL that can be used to fetch the given file.
//
// 'gsPath' should have form '/bucket/path'. 'filename', if given, will be
// returned in Content-Disposition header when accessing the signed URL. It
// instructs user agents to save the file under the given name.
//
// 'signAs' is an email of a service account to impersonate when sining or ""
// to use the default service account.
//
// The returned URL is valid for at least 30 min (may be longer). It's expected
// that it will be used right away, not stored somewhere.
//
// On failures returns grpc-annotated errors. In particular, if the requested
// file is missing, returns NotFound grpc-annotated error.
func getSignedURL(c context.Context, gsPath, signAs, filename string) (string, error) {
	cached, err := signedURLsCache.GetOrCreate(c, gsPath, func() (interface{}, time.Duration, error) {
		switch yes, err := gs.Get(c).Exists(c, gsPath); {
		case err != nil:
			return nil, 0, errors.Annotate(err, "failed to check GS file presence").Err()
		case !yes:
			return "", time.Minute, nil // cache the absence for 1 min
		}

		url, err := signURL(c, gsPath, signAs, maxSignedURLExpiration)
		if err != nil {
			return nil, 0, errors.Annotate(err, "failed to sign GS URL").Err()
		}

		return url, maxSignedURLExpiration - minSignedURLExpiration, nil
	})
	if err != nil {
		return "", errors.Annotate(err, "failed to sign URL").
			Tag(grpcutil.InternalTag).Err()
	}

	signedURL := cached.(string)
	if signedURL == "" {
		return "", errors.Reason("object %q doesn't exist", gsPath).
			Tag(grpcutil.NotFoundTag).Err()
	}

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

	return signedURL, nil
}

// signURL generates a signed GS URL using the signer in the context.
func signURL(c context.Context, gsPath, signAs string, expiry time.Duration) (string, error) {
	var signerID string
	var signer signer
	var err error
	if signAs == "" {
		signerID, signer, err = defaultSigner(c)
	} else {
		signerID, signer, err = iamSigner(c, signAs)
	}
	if err != nil {
		return "", errors.Annotate(err, "can't get the signer").Err()
	}

	// See https://cloud.google.com/storage/docs/access-control/signed-urls.
	//
	// Basically, we should sign a specially crafted multi-line string that
	// encodes expected parameters of the request. During the actual request,
	// Google Storage backend will construct the same string and verify that
	// provided signature matches it.

	expires := fmt.Sprintf("%d", clock.Now(c).Add(expiry).Unix())

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "GET\n")
	fmt.Fprintf(buf, "\n") // expected value of 'Content-MD5' header, not used
	fmt.Fprintf(buf, "\n") // expected value of 'Content-Type' header, not used
	fmt.Fprintf(buf, "%s\n", expires)
	fmt.Fprintf(buf, "%s", gsPath)

	_, sig, err := signer.SignBytes(c, buf.Bytes())
	if err != nil {
		return "", errors.Annotate(err, "failed to sign the URL").Err()
	}

	u := url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   gsPath,
		RawQuery: (url.Values{
			"GoogleAccessId": {signerID},
			"Expires":        {expires},
			"Signature":      {base64.StdEncoding.EncodeToString(sig)},
		}).Encode(),
	}
	return u.String(), nil
}

// signer can RSA-sign blobs the way Google Storage likes it.
type signer interface {
	SignBytes(context.Context, []byte) (key string, sig []byte, err error)
}

// defaultSigner uses the default app account for signing.
func defaultSigner(c context.Context) (id string, s signer, err error) {
	signer := auth.GetSigner(c)
	if signer == nil {
		return "", nil, errors.Reason("a default signer is not available").Err()
	}
	info, err := signer.ServiceInfo(c)
	if err != nil {
		return "", nil, errors.Annotate(err, "failed to grab the signer info").Err()
	}
	return info.ServiceAccountName, signer, nil
}

// iamSigner uses SignBytes IAM API for signing.
func iamSigner(c context.Context, actAs string) (id string, s signer, err error) {
	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(iam.OAuthScope))
	if err != nil {
		return "", nil, errors.Annotate(err, "failed to grab RPC transport").Err()
	}
	return actAs, &iam.Signer{
		Client:         &iam.Client{Client: &http.Client{Transport: t}},
		ServiceAccount: actAs,
	}, nil
}
