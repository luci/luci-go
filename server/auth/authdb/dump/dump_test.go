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

package dump

import (
	"context"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/retry"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetcher(t *testing.T) {
	t.Parallel()

	signer := signingtest.NewSigner(nil)

	Convey("With mocks", t, func(c C) {
		serverAllowAccess := true           // is caller allowed to access authdb?
		serverHasAccessNow := false         // does caller have access right now?
		serverDumpPath := "bucket/prefix"   // where server dumps AuthDB
		serverSignerID := "auth-service-id" // ID used to sign blob by the server
		serverSignature := []byte(nil)      // if non-nil, mocked signature

		serverLatestRev := int64(1234) // revision of the latest dump
		serverLatestVal := "latest"    // payload in the latest dump

		ctx := authtest.MockAuthConfig(context.Background())

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth_service/api/v1/authdb/subscription/authorization":
				if !serverAllowAccess {
					w.WriteHeader(403)
				} else {
					serverHasAccessNow = true
					w.Write([]byte(fmt.Sprintf(`{"gs":{"auth_db_gs_path":%q}}`, serverDumpPath)))
				}
			case "/bucket/prefix/latest.json":
				if !serverHasAccessNow {
					w.WriteHeader(403)
				} else {
					w.Write([]byte(fmt.Sprintf(`{"auth_db_rev": "%d"}`, serverLatestRev)))
				}
			case "/bucket/prefix/latest.db":
				if !serverHasAccessNow {
					w.WriteHeader(403)
				} else {
					w.Write(genSignedAuthDB(ctx, signer,
						serverSignerID, serverSignature, serverLatestRev, serverLatestVal))
				}
			default:
				c.So(r.URL, ShouldEqual, "")
			}
		}))
		defer ts.Close()

		signingCerts, err := signer.Certificates(ctx)
		So(err, ShouldBeNil)

		f := Fetcher{
			StorageDumpPath:    "bucket/prefix",
			AuthServiceURL:     ts.URL,
			AuthServiceAccount: serverSignerID,
			OAuthScopes:        []string{"scope1", "scope2"},

			testRetryPolicy:   func() retry.Iterator { return &retry.Limited{Retries: 0} },
			testStorageURL:    ts.URL,
			testStorageClient: http.DefaultClient,
			testSigningCerts:  signingCerts,
		}

		Convey("Fetching works", func() {
			db, err := f.FetchAuthDB(ctx, nil)
			So(err, ShouldBeNil)
			assertAuthDB(db, serverLatestRev, serverLatestVal)

			Convey("Skips updating if nothing has changed", func() {
				fetched, err := f.FetchAuthDB(ctx, db)
				So(err, ShouldBeNil)
				So(fetched == db, ShouldBeTrue) // the exact same object
			})

			Convey("Updates if the revision goes up", func() {
				serverLatestRev++
				serverLatestVal = "newer"

				fetched, err := f.FetchAuthDB(ctx, db)
				So(err, ShouldBeNil)
				assertAuthDB(fetched, serverLatestRev, serverLatestVal)
			})

			Convey("Refuses to update if the revision goes down", func() {
				serverLatestRev--
				serverLatestVal = "older"

				fetched, err := f.FetchAuthDB(ctx, db)
				So(err, ShouldBeNil)
				So(fetched == db, ShouldBeTrue) // the exact same object
			})
		})

		Convey("Not authorized", func() {
			serverAllowAccess = false
			_, err := f.FetchAuthDB(ctx, nil)
			So(err, ShouldErrLike, "HTTP code (403)")
		})

		Convey("Wrong storage path", func() {
			serverDumpPath = "something/else"
			_, err := f.FetchAuthDB(ctx, nil)
			So(err, ShouldErrLike, "wrong configuration")
		})

		Convey("Unexpected signer", func() {
			serverSignerID = "someone-else"
			_, err := f.FetchAuthDB(ctx, nil)
			So(err, ShouldErrLike, "the snapshot is signed by")
		})

		Convey("Bad signature", func() {
			serverSignature = []byte("bad signature")
			_, err := f.FetchAuthDB(ctx, nil)
			So(err, ShouldErrLike, "failed to verify that AuthDB was signed")
		})
	})
}

// genSignedAuthDB generates and signs auth DB blob.
func genSignedAuthDB(ctx context.Context, signer *signingtest.Signer, signerID string, sig []byte, rev int64, data string) []byte {
	authDB, err := proto.Marshal(&protocol.ReplicationPushRequest{
		Revision: &protocol.AuthDBRevision{AuthDbRev: rev},
		// We abuse TokenServerUrl to pass some payload checked by assertAuthDB.
		AuthDb: &protocol.AuthDB{TokenServerUrl: data},
	})
	if err != nil {
		panic(err)
	}

	hash := sha512.Sum512(authDB)
	keyID, realSig, err := signer.SignBytes(ctx, hash[:])
	if err != nil {
		panic(err)
	}
	if sig == nil {
		sig = realSig
	}

	blob, err := proto.Marshal(&protocol.SignedAuthDB{
		AuthDbBlob:   authDB,
		SignerId:     signerID,
		SigningKeyId: keyID,
		Signature:    sig,
	})
	if err != nil {
		panic(err)
	}
	return blob
}

// assertAuthDB verifies 'db' was constructed from output of genSignedAuthDB.
func assertAuthDB(db *authdb.SnapshotDB, rev int64, data string) {
	So(db.Rev, ShouldEqual, rev)
	d, _ := db.GetTokenServiceURL(nil)
	So(d, ShouldEqual, data)
}
