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

type fakeGenerator struct {
	accessToken *oauth2.Token
	idToken     string
}

func (f *fakeGenerator) GenerateOAuthToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	return f.accessToken, nil
}

func (f *fakeGenerator) GenerateIDToken(ctx context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: f.idToken}, nil
}

func TestServer(t *testing.T) {
	fakeAccessToken := &oauth2.Token{
		AccessToken: "fake_access_token",
		Expiry:      time.Now().Add(time.Hour),
	}
	fakeIDToken := "fake_id_token"

	ctx := context.Background()
	srv := Server{
		Generator:        &fakeGenerator{fakeAccessToken, fakeIDToken},
		Email:            "fake@example.com",
		Scopes:           []string{"scope1", "scope2"},
		MinTokenLifetime: 2 * time.Minute,

		md: &serverMetadata{
			zone: "luci-emulated-zone",
			name: "luci-emulated",
		},
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

			zone, err := cl.Zone()
			So(err, ShouldBeNil)
			So(zone, ShouldEqual, "luci-emulated-zone")

			name, err := cl.InstanceName()
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "luci-emulated")

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

		Convey("OAuth2 token source works", func() {
			ts := google.ComputeTokenSource("default", "ignored-scopes")
			tok, err := ts.Token()
			So(err, ShouldBeNil)
			// Do not put tokens into logs, in case we somehow accidentally hit real
			// metadata server with real tokens.
			if tok.AccessToken != fakeAccessToken.AccessToken {
				panic("Bad token")
			}
			So(time.Until(tok.Expiry), ShouldBeGreaterThan, 55*time.Minute)
		})

		Convey("ID token fetch works", func() {
			reply, err := metadata.Get("instance/service-accounts/default/identity?audience=boo&format=ignored")
			So(err, ShouldBeNil)
			// Do not put tokens into logs, in case we somehow accidentally hit real
			// metadata server with real tokens.
			if reply != fakeIDToken {
				panic("Bad token")
			}
		})

		Convey("Unsupported metadata call", func() {
			_, err := metadata.InstanceID()
			So(err.Error(), ShouldEqual, `metadata: GCE metadata "instance/id" not defined`)
		})
	})
}
