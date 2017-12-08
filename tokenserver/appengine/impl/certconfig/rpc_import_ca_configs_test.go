// Copyright 2016 The LUCI Authors.
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

package certconfig

import (
	"testing"

	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"

	"go.chromium.org/luci/tokenserver/api/admin/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImportCAConfigsRPC(t *testing.T) {
	Convey("with mock context", t, func() {
		ctx := gaetesting.TestingContext()

		callImport := func(cfg string) (*admin.ImportedConfigs, error) {
			impl := ImportCAConfigsRPC{}
			return impl.ImportCAConfigs(prepareCfg(ctx, cfg), nil)
		}

		getCA := func(cn string) *admin.GetCAStatusResponse {
			impl := GetCAStatusRPC{}
			resp, err := impl.GetCAStatus(ctx, &admin.GetCAStatusRequest{
				Cn: cn,
			})
			if err != nil {
				panic(err)
			}
			return resp
		}

		listCAs := func() []string {
			impl := ListCAsRPC{}
			resp, err := impl.ListCAs(ctx, nil)
			if err != nil {
				panic(err)
			}
			return resp.Cn
		}

		Convey("dry run", func() {
			_, err := callImport("")
			So(err, ShouldBeNil)
		})

		Convey("import one CA, update it", func() {
			// Nothing there.
			So(getCA("Puppet CA: fake.ca").Config, ShouldBeNil)

			// Import.
			out, err := callImport(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
				}
			`)
			So(err, ShouldBeNil)
			firstRev := out.Revision

			// Appears.
			resp := getCA("Puppet CA: fake.ca")
			So(resp.Config, ShouldNotBeNil)
			So(resp.Cert, ShouldEqual, fakeCACrt)
			So(resp.AddedRev, ShouldEqual, firstRev)
			So(resp.UpdatedRev, ShouldEqual, firstRev)

			// Noop import.
			out, err = callImport(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					# some comment
					cert_path: "certs/fake.ca.crt"
				 }
			`)
			So(err, ShouldBeNil)
			So(out.Revision, ShouldNotEqual, firstRev)

			// UpdateRev stays as it was, no significant changes made.
			resp = getCA("Puppet CA: fake.ca")
			So(resp.UpdatedRev, ShouldEqual, firstRev)

			// Change config for real now.
			out, err = callImport(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
					crl_url: "https://blah"
				 }
			`)
			So(err, ShouldBeNil)
			secondRev := out.Revision

			// Assert it is updated.
			resp = getCA("Puppet CA: fake.ca")
			So(resp.UpdatedRev, ShouldEqual, secondRev)
			So(resp.Config.CrlUrl, ShouldEqual, "https://blah")
		})

		Convey("add one, replace with another", func() {
			// Nothing there.
			So(listCAs(), ShouldBeEmpty)

			// Import fake.ca first.
			_, err := callImport(`
				 certificate_authority {
					cn: "Puppet CA: fake.ca"
					unique_id: 1
					cert_path: "certs/fake.ca.crt"
				}
			`)
			So(err, ShouldBeNil)

			ds.GetTestable(ctx).CatchupIndexes()

			// Appears.
			So(listCAs(), ShouldResemble, []string{"Puppet CA: fake.ca"})

			// Replace it with another-fake.ca.
			out, err := callImport(`
				certificate_authority {
					cn: "Puppet CA: another-fake.ca"
					unique_id: 2
					cert_path: "certs/another-fake.ca.crt"
				}
			`)
			So(err, ShouldBeNil)
			rev := out.Revision

			ds.GetTestable(ctx).CatchupIndexes()

			// fake.ca is removed.
			resp := getCA("Puppet CA: fake.ca")
			So(resp.Removed, ShouldBeTrue)
			So(resp.RemovedRev, ShouldEqual, rev)

			// another-fake.ca is added.
			resp = getCA("Puppet CA: another-fake.ca")
			So(resp.AddedRev, ShouldEqual, rev)

			// Listing shows only active CAs.
			So(listCAs(), ShouldResemble, []string{"Puppet CA: another-fake.ca"})
		})

		Convey("rejects duplicates", func() {
			_, err := callImport(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
				}
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
					crl_url: "http://blah"
				}
			`)
			So(err, ShouldErrLike, "duplicate entries in the config")
		})

		Convey("rejects wrong CN", func() {
			_, err := callImport(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/another-fake.ca.crt"
				}
			`)
			So(err, ShouldErrLike, "bad CN in the certificat")
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

// Valid CA cert with CN "Puppet CA: another-fake.ca".
const anotherFakeCACrt = `-----BEGIN CERTIFICATE-----
MIIFeTCCA2GgAwIBAgIBATANBgkqhkiG9w0BAQsFADAlMSMwIQYDVQQDDBpQdXBw
ZXQgQ0E6IGFub3RoZXItZmFrZS5jYTAeFw0xNjAzMTcwMzE4NDdaFw0yMTAzMTcw
MzE4NDdaMCUxIzAhBgNVBAMMGlB1cHBldCBDQTogYW5vdGhlci1mYWtlLmNhMIIC
IjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAq7kdWmo8zRNbhR0eOaDe7nFP
fuJEUXMObYg/3+qGv/NyPJQEFa4B6UxpkNzU4wshsS9r73P2kTCM5Ix1hFfBgJ6h
LIPnsQA0a0O7dao5mazeqm01aEO1NhDTP9K0fGkmRO4qoKw/GvDL5TCOcUVQkdq4
Hf9RXc56yI3o7BLoZu5KrfcO72OLINZ5puEramOjc/v/b/Ri7F5ldn/3btfJ3Wj9
VxBNY5kUf8bqPM9wA4lDF31zbQtpHv7+va78zpYgFnokOzTxqk7kGQs1HCbmkJe9
pkkOmpd/4CFOxq9SBGnXT/xwVFFdID1QksMjZw1x044lj2kGTy1h/sc0PA82dDni
ooAqxVcs59T272zLNawSeFxZINnQWzcjnBrdv2rvz2QTRIvL6CEudAaDQWVSeJj9
m4b71sGe715/FgoAcqVvxhTYjBDSEjwkrX6WFDzz13w88wQ3byat4u+BgHCdtkod
QT31P7hZlKlsPjjCxF3xX55GA88rCvd1ppv7mRGei4hModY+JxHH9YABQ6S+ZsKS
XnJAVPfxlSCzzylHUYl4lUNDHN1CNbl4z/X8Zd4n4u88wnf6EGufpLgWw4jds1EF
eB2UwMjNg74jhXIFVBkUN4zicAk3ahGfh1GUgZ/BhONDy1RqOB4kFlAxhU+X09qC
WGNL6xEvaFljnFVSEasCAwEAAaOBszCBsDA1BglghkgBhvhCAQ0EKFB1cHBldCBS
dWJ5L09wZW5TU0wgSW50ZXJuYWwgQ2VydGlmaWNhdGUwDgYDVR0PAQH/BAQDAgEG
MA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFK3M9neU7Z73Fg/yYSO4h/qZOIrA
MDcGA1UdIwQwMC6hKaQnMCUxIzAhBgNVBAMMGlB1cHBldCBDQTogYW5vdGhlci1m
YWtlLmNhggEBMA0GCSqGSIb3DQEBCwUAA4ICAQAXYL/OFaV9g59X2ZIvz93ogMl8
R/nhpYmG0yA9ylqqvFq/3Sht+pRML0BUHvJINzvfuEscO0IzFOr1L0DQuRwtkszX
bN9K1w9tGwHOcoqEsHL4FC3p6LVnSNP18xh45MvaGU6pgscZX7KG/VCHFGH9+1kq
lkImNyI3sn5+OPoa9Y0ge32dENn9YMbwU+vqNMbgAnnkGDUl6xfUNlt2vb4HSpR9
6pnosRBFyqx3stXsAjlLHk4cp2HKl3G8AMkSEs0z0ALZv5m7/fJ5b4e285jGSxhd
CEKCFVRB/c+RPwfgW2k9fCIKUZYZD+uzok3I6U0ycEiazKtWLmrOd3J+ltqY1hTK
ZRn8T2pCm4i+oVE0ObPNRhcHMDWIGDuFpVKk4Hi/8h00EcvoJRRKSj5yIReVURLX
FNhRTP7aoGf7ktzOH2frOtyKxFO49BXgq43dmLNIzLA/kSWnhSvzxVzSEtZALV4q
OU7SevV9eJ7oeWhDdMmlVwWgH4upTqEseAcs28JpN9UaSDLsnQloX4s1VIJ0Pbi7
OA//8e4Tx7mXU21Pr/3Ek3QB4vwWeo9n9wPd8vz/Y5YjV1e1Qw+Ey7jk3thBoAyu
JzP7+U1dTrpLTi0souhc4f2OSJ9v3QRFFBHB2Yfbze+swmF9VPcMcazaelXnV1zH
PkoYH9WC8tSbqNof3g==
-----END CERTIFICATE-----
`

// prepareCfg injects config.Backend implementation with a bunch of
// config files.
func prepareCfg(c context.Context, configFile string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[string]memory.ConfigSet{
		"services/" + info.AppID(c): {
			"tokenserver.cfg":           configFile,
			"certs/fake.ca.crt":         fakeCACrt,
			"certs/another-fake.ca.crt": anotherFakeCACrt,
		},
	}))
}
