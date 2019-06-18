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

package gcemeta

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServer(t *testing.T) {
	fakeToken := &oauth2.Token{
		AccessToken: "fake_token",
		Expiry:      time.Now().Add(time.Hour),
	}

	ctx := context.Background()
	srv := Server{
		Source: oauth2.StaticTokenSource(fakeToken),
		Email:  "fake@example.com",
		Scopes: []string{"scope1", "scope2"},
	}

	// Need to set GCE_METADATA_HOST once before all tests because 'metadata'
	// package caches results of metadata fetches.
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %s", err)
	}
	defer srv.Stop(ctx)

	os.Setenv("GCE_METADATA_HOST", addr)

	Convey("Works", t, func(c C) {
		So(metadata.OnGCE(), ShouldBeTrue)

		Convey("Metadata client works", func() {
			cl := metadata.NewClient(http.DefaultClient)

			num, err := cl.NumericProjectID()
			So(err, ShouldBeNil)
			So(num, ShouldEqual, "0")

			pid, err := cl.ProjectID()
			So(err, ShouldBeNil)
			So(pid, ShouldEqual, "none")

			accounts, err := cl.Get("instance/service-accounts/")
			So(err, ShouldBeNil)
			So(accounts, ShouldEqual, "fake@example.com/\ndefault/\n")

			for _, acc := range []string{"fake@example.com", "default"} {
				info, err := cl.Get("instance/service-accounts/" + acc + "/?recursive=true")
				So(err, ShouldBeNil)
				So(info, ShouldEqual,
					`{"aliases":["default"],"email":"fake@example.com","scopes":["scope1","scope2"]}`+"\n")

				email, err := cl.Get("instance/service-accounts/" + acc + "/email")
				So(err, ShouldBeNil)
				So(email, ShouldEqual, "fake@example.com")

				scopes, err := cl.Scopes(acc)
				So(err, ShouldBeNil)
				So(scopes, ShouldResemble, []string{"scope1", "scope2"})
			}
		})

		Convey("Token source works", func() {
			ts := google.ComputeTokenSource("default", "ignored-scopes")
			tok, err := ts.Token()
			So(err, ShouldBeNil)
			// Do not put tokens into logs, in case we somehow accidentally hit real
			// metadata server with real tokens.
			if tok.AccessToken != fakeToken.AccessToken {
				panic("Bad token")
			}
			So(time.Until(tok.Expiry), ShouldBeGreaterThan, 55*time.Minute)
		})

		Convey("Unsupported metadata call", func() {
			_, err := metadata.InstanceID()
			So(err.Error(), ShouldEqual, `metadata: GCE metadata "instance/id" not defined`)
		})
	})
}
