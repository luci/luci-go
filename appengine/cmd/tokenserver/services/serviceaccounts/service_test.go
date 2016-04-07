// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serviceaccounts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/gaetesting"
	"google.golang.org/api/iam/v1"

	"github.com/luci/luci-go/appengine/cmd/tokenserver/model"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/utils"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/google"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCreateServiceAccount(t *testing.T) {
	Convey("works", t, func(c C) {
		router := httprouter.New()

		createCalls := 0

		router.POST("/v1/projects/cloud-project/serviceAccounts",
			func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
				createCalls++

				body := readJSON(r)
				c.So(jsonValue(body, "accountId"), ShouldResemble, "some-host")
				c.So(jsonValue(body, "serviceAccount", "displayName"), ShouldResemble, "some-host.fake.domain")

				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				rw.Write([]byte(`{
					"name": "projects/cloud-project/serviceAccounts/some-host@cloud-project.iam.gserviceaccount.com",
					"projectId": "cloud-project",
					"uniqueId": "12345",
					"email": "some-host@cloud-project.iam2.gserviceaccount.com",
					"displayName": "some-host.fake.domain",
					"etag": "blah",
					"oauth2ClientId": "12345"
				}`))
			})

		ctx, srv, closer := setupTest(router)
		defer closer()

		resp, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "SOME-HOST.FAKE.DOMAIN",
		})
		So(err, ShouldBeNil)
		resp.ServiceAccount.Registered = nil // don't care
		So(resp.ServiceAccount, ShouldResemble, &tokenserver.ServiceAccount{
			ProjectId:      "cloud-project",
			UniqueId:       "12345",
			Email:          "some-host@cloud-project.iam2.gserviceaccount.com",
			DisplayName:    "some-host.fake.domain",
			Oauth2ClientId: "12345",
			Fqdn:           "some-host.fake.domain",
		})
		So(createCalls, ShouldEqual, 1)

		// Idempotent. Doesn't call IAM API again.
		resp, err = srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "SOME-HOST.FAKE.DOMAIN",
		})
		So(err, ShouldBeNil)
		So(createCalls, ShouldEqual, 1)
	})

	Convey("handled conflict", t, func(c C) {
		router := httprouter.New()

		router.POST("/v1/projects/cloud-project/serviceAccounts",
			func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusConflict)
			})

		router.GET("/v1/projects/cloud-project/serviceAccounts/some-host@cloud-project.iam.gserviceaccount.com",
			func(rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				rw.Write([]byte(`{
					"name": "projects/cloud-project/serviceAccounts/some-host@cloud-project.iam.gserviceaccount.com",
					"projectId": "cloud-project",
					"uniqueId": "12345",
					"email": "some-host@cloud-project.iam.gserviceaccount.com",
					"displayName": "some-host.fake.domain",
					"etag": "blah",
					"oauth2ClientId": "12345"
				}`))
			})

		ctx, srv, closer := setupTest(router)
		defer closer()

		_, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "SOME-HOST.FAKE.DOMAIN",
		})
		So(err, ShouldBeNil)
	})

	Convey("unknown CA", t, func(c C) {
		ctx, srv, _ := setupTest(nil)
		_, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: unknown",
			Fqdn: "SOME-HOST.FAKE.DOMAIN",
		})
		So(err, ShouldErrLike, "error when fetching CA config - no such CA")
	})

	Convey("bad FQDN", t, func(c C) {
		ctx, srv, _ := setupTest(nil)
		_, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "SOME-HOST",
		})
		So(err, ShouldErrLike, "not a valid FQDN")
	})

	Convey("not whitelisted domain", t, func(c C) {
		ctx, srv, _ := setupTest(nil)
		_, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "SOME-HOST.unknown.domain",
		})
		So(err, ShouldErrLike, "not whitelisted in the config")
	})

	Convey("too short", t, func(c C) {
		ctx, srv, _ := setupTest(nil)
		_, err := srv.CreateServiceAccount(ctx, &tokenserver.CreateServiceAccountRequest{
			Ca:   "Puppet CA: fake.ca",
			Fqdn: "123.fake.domain",
		})
		So(err, ShouldErrLike, "and be 6-30 characters long")
	})
}

func TestMintAccessToken(t *testing.T) {
	Convey("works", t, func(c C) {
		router := httprouter.New()

		router.POST("/v1/projects/cloud-project/serviceAccounts/:rest",
			func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
				rest := p.ByName("rest")
				if rest != "account@cloud-project.iam.gserviceaccount.com:signBlob" {
					rw.WriteHeader(http.StatusNotFound)
					return
				}
				body := readJSON(r)

				jwtToSign, _ := base64.StdEncoding.DecodeString(jsonValue(body, "bytesToSign").(string))
				chunks := strings.Split(string(jwtToSign), ".")
				c.So(len(chunks), ShouldEqual, 2)
				c.So(chunks[0], ShouldEqual, "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9") // JWT header

				payload, _ := base64.RawStdEncoding.DecodeString(chunks[1])
				c.So(string(payload), ShouldEqual,
					`{"iss":"account@cloud-project.iam.gserviceaccount.com",`+
						`"scope":"scope1 scope2",`+
						`"aud":"https://www.googleapis.com/oauth2/v4/token",`+
						`"exp":1422939896,"iat":1422936296}`)

				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				rw.Write(
					[]byte(fmt.Sprintf(`{"signature":"%s"}`,
						base64.StdEncoding.EncodeToString([]byte("signature")))))
			})

		router.POST("/fake-token-endpoint",
			func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
				r.ParseForm()
				c.So(r.FormValue("grant_type"), ShouldEqual, jwtGrantType)
				c.So(r.FormValue("assertion"), ShouldEqual, "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9."+
					"eyJpc3MiOiJhY2NvdW50QGNsb3VkLXByb2plY3QuaWFtLmdzZXJ2aWNlYWNjb3VudC"+
					"5jb20iLCJzY29wZSI6InNjb3BlMSBzY29wZTIiLCJhdWQiOiJodHRwczovL3d3dy5n"+
					"b29nbGVhcGlzLmNvbS9vYXV0aDIvdjQvdG9rZW4iLCJleHAiOjE0MjI5Mzk4OTYsIm"+
					"lhdCI6MTQyMjkzNjI5Nn0.c2lnbmF0dXJl")
				var tokenRes struct {
					AccessToken string `json:"access_token"`
					TokenType   string `json:"token_type"`
					IDToken     string `json:"id_token"`
					ExpiresIn   int64  `json:"expires_in"` // relative seconds from now
				}
				tokenRes.AccessToken = "access_token"
				tokenRes.TokenType = "Bearer"
				tokenRes.ExpiresIn = 3600
				blob, _ := json.Marshal(&tokenRes)
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				rw.Write(blob)
			})

		ctx, srv, closer := setupTest(router)
		defer closer()

		// Pretend service account already exists.
		_, err := storeAccountInfo(ctx, "cloud-project", "account", "account.fake.domain", &iam.ServiceAccount{
			Email: "account@cloud-project.iam.gserviceaccount.com",
		})
		So(err, ShouldBeNil)

		// Use it to mint a token.
		resp, err := srv.MintAccessToken(ctx, &tokenserver.MintAccessTokenRequest{
			Ca:     "Puppet CA: fake.ca",
			Fqdn:   "account.fake.domain",
			Scopes: []string{"scope2", "scope1"},
		})
		So(err, ShouldBeNil)
		So(resp.ServiceAccount, ShouldResemble, &tokenserver.ServiceAccount{
			ProjectId: "cloud-project",
			Email:     "account@cloud-project.iam.gserviceaccount.com",
			Fqdn:      "account.fake.domain",
			Registered: &google.Timestamp{
				Seconds: 1422936306,
				Nanos:   0,
			},
		})
		So(resp.Oauth2AccessToken, ShouldResemble, &tokenserver.OAuth2AccessToken{
			AccessToken: "access_token",
			TokenType:   "Bearer",
			Expiry: &google.Timestamp{
				Seconds: 1422939906,
				Nanos:   7, // huh?
			},
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

// Valid CA cert with CN "Puppet CA: fake.ca".
const fakeCACrt = `-----BEGIN CERTIFICATE-----
MIIFYTCCA0mgAwIBAgIBATANBgkqhkiG9w0BAQsFADAdMRswGQYDVQQDDBJQdXBw
ZXQgQ0E6IGZha2UuY2EwHhcNMTYwMzE0MDE0NTIyWhcNMjEwMzE0MDE0NTIyWjAd
MRswGQYDVQQDDBJQdXBwZXQgQ0E6IGZha2UuY2EwggIiMA0GCSqGSIb3DQEBAQUA
A4ICDwAwggIKAoICAQC4seou44kS+nMB2sqacLWlBqavMDcVqA4YHMnMNA6BzMVm
vsLP88/uYAlVwLH7oovMrpHoq8SlD0xwKovs02Upa2OUdgNOKdCiOxTzRWjlx0Zr
cSeXGfph5d/7lytcL2OJubXzgcDpCOzOSvECWSCl0rjJ939bUqffwE/uCKHau42g
WXdo/ubkQhHri5AGlzD1gqAO5HTeUASJ5m/cijtAhtySRrDQrRMUaX+/1/QSdHQb
zbP8MvrZH85lRqFsd82UnANRMS5709P9RHXVg+CiyOMyj9a0AvX1eXwGueGv8eVa
7bEpkP4aSB5EccC/5wSkOmlHnPehRKDN1a6SOADE/f8xJ0o6WVoSqgSC5TYFiiSL
DGF7j4ppJE8akXdVrDJ1EY7ABBK8pgFbto+B3U88rSx3UFON+Wmz2UQue875cNlw
86ENg0sl6nFqi7tdajOAuLYce4cPipOu+hQVBOtqsdhlnpquKH3tbtV3mIyeg1pf
R90idwvpGTVVdR/XH+p5s9XrT+bI/wec/VwC0Djs2ZEyiy84nLgXT5wV/CEqAxeo
7T9gA5YVO7kMk0Q47Hnl1yhukiSWt5B4vWezO+jZt6mrQz6lFeHmoiT0U062vttO
1e0JPPCXbqRQ94q+wP21lxRvlMmBa3TV6+JZRU+2o4v1aIZ6B0Cprog7+8a1uQID
AQABo4GrMIGoMDUGCWCGSAGG+EIBDQQoUHVwcGV0IFJ1YnkvT3BlblNTTCBJbnRl
cm5hbCBDZXJ0aWZpY2F0ZTAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB
/zAdBgNVHQ4EFgQU54Y/U6x72ym+EgisYwRkSmh6IOowLwYDVR0jBCgwJqEhpB8w
HTEbMBkGA1UEAwwSUHVwcGV0IENBOiBmYWtlLmNhggEBMA0GCSqGSIb3DQEBCwUA
A4ICAQBYkYF7gRFWFV+mp7+vrSwFpBVtceonp5Aq8OtiiZBeRZsBKPWOyfq/tMmu
TPKy3SgPYTFwZTUymRvrBOGBd1n+5qblAkfjSpvirUoWP6HezsEpU/8x4UqK8PcE
tjcMiUPub7lyyNZap2tU88Oj/6tk+1JKwcJp3AKkI8fcHkmYUDlPDb60/QH5bln0
4sAr8FXeSACWv6asn738lDYt2DrlkseY+M6rUy3UQ97f6ESYbB655dfFQGSWnIOt
XXChCB+9hB4boXkuvHBqZ4ww/tum/sC/aO15KfXP9HRba8IqgmaBn5H26sN8BJye
8Ly359SKwyrRNNC85A528xJz98mgj25gQVXCYbMeln7MbnEg3MmOI4Ky82AWIz1F
P9fN5ISmEQCChBGENm1p9W1PkyL28vvNvmWswgufp8DUpuGSS7OQAyxJVTVcxk4W
Qft6giSElo1o5Xw3KnxXWKQuF1fKv8Y7scDNEhC4BRTiYYLT1bnbVm7welcWqiWf
WtwPYghRtj166nPfnpxPexxN+aR6055c8Ot+0wdx2tPrTStVv9yL9oXTVBcHXy3l
a9S+6vGE2c+cpXhnDXXB6mg/co2UmhCoY39doUbJyPlzf0sv+k/8lPGbo84qlJMt
thi7LhTd2md+7zzukdrl6xdqYwZXTili5bEveVERajRTVhWKMg==
-----END CERTIFICATE-----
`

func setupTest(fakes http.Handler) (context.Context, *Server, func()) {
	var closer func()

	var tsURL string
	if fakes != nil {
		ts := httptest.NewServer(fakes)
		closer = ts.Close
		tsURL = ts.URL
	} else {
		closer = func() {}
	}

	srv := &Server{
		transport:       http.DefaultTransport,
		iamBackendURL:   tsURL,
		tokenBackendURL: tsURL + "/fake-token-endpoint",
		ownEmail:        "token-server@fake.gserviceaccount.com",
	}

	ctx := gaetesting.TestingContext()
	ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 7, time.UTC))

	// Put fake config.
	cfg := tokenserver.CertificateAuthorityConfig{
		KnownDomains: []*tokenserver.DomainConfig{
			{
				Domain:             []string{"fake.domain"},
				CloudProjectName:   "cloud-project",
				AllowedOauth2Scope: []string{"scope1", "scope2"},
			},
		},
	}
	blob, err := proto.Marshal(&cfg)
	if err != nil {
		panic(err)
	}
	certDer, _ := utils.ParsePEM(fakeCACrt, "CERTIFICATE")
	caEntity := model.CA{
		CN:     "Puppet CA: fake.ca",
		Cert:   certDer,
		Ready:  true,
		Config: blob,
	}
	if err = datastore.Get(ctx).Put(&caEntity); err != nil {
		panic(err)
	}

	return ctx, srv, closer
}

func readJSON(r *http.Request) map[string]interface{} {
	out := make(map[string]interface{})
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		panic(err)
	}
	return out
}

func jsonValue(j interface{}, path ...string) interface{} {
	cur := j
	for _, k := range path {
		asMap, _ := cur.(map[string]interface{})
		cur = asMap[k]
	}
	return cur
}
