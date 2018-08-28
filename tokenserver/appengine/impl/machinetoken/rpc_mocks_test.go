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

package machinetoken

// This file contains common mocks used by unit tests.

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	minter "go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

const pkey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAvc6v42I4badqYA+IF9dMB838Q2l2IflSpA8xSC5O7XrDwa1R
YCqPq+MOIIaMUgqBMJz0OmNyQkbtRLq3Qu4Q44UIbdqyy34rj7kcw/9t/K9x+2ne
Phx0tfdz+5Lj6UiRRI7FRCi9cs+mgSQquCDaBW8J5l3lCZEne8fpHPO3Hxl+dkUX
0Y8T2ZCsn19hnV7Z7wbfN1dUuRihXD+UwN2axoqZ0EJ2GNSLYAz3HHkKc6ELM1Lx
biOD9Jxw8wb+5VnpIuR3l426Fcux9EQGewLZFTxjRS7DRPL/9L0xE+yRJ/I04UyJ
v4Ws98fYp/vAM922Wt21P9Py6vgn+Xjyz2AoyQIDAQABAoIBABKQhq+Mycwf1c2z
dzItwqf4w7WsOPu1sRmOytkuflXH7iGhXBY103uSZ61Su6LCmEQy9chINcK5wTc5
s/b95fT67Aoim94/Zu9VwbSB5TYTyug2QKB+lAPAQj3W7ifBC0RTWoQCIBV8reJv
sSX1QJ3LcIJxqJc49U2sDebhB4YpAv7xmy4IfpqX+0iURtXrgBmp0hWKTQb7kRdG
BycDU9/AadgkI1PEhRdbfJ4VHFKxkeSRwPyp1UKvzydfe7Nw2HWlflEH4fZCc29x
AM52K5zN/7ns/xEz9XPOUG0/pBcXeQNA5rbTGoZrhQda/aBWbI9TYGWh7XZFvx5y
vZ/xlckCgYEA6ULnJYn+DDUfa1eKwYsYvzf82pbfBvrpTsIKi9h0CWy7GltYuSJk
6yt6pzEmAeV+eY6/F8pPxN0TTZHQAVcRHiMbazmLiaLUeuZCvIZwU54ttyENC2+k
fLUlt3a5eiPKBZEPGx++HuESWVY0LYk8hcg9koc4+AIsiifXz9kgzRcCgYEA0E9h
Dn1qWEXYxzV6Efcux62d9P8vwRx+ZWtbCdMleosVAWQ4RtS8HKfjYvJdPcKJlaGO
b7VyYbJzayPs0skhayEYXajDhwcykxNCYJTXxSqh3Hf4yEeRLquDLW468a9tRc8q
Q2wv+lav7ZeW+Db35fq0mEHRaUn0iXFiq9c1JR8CgYEA16ocrk98TGsdRpCk4Lcr
RTiNlsihIgIAjenH+G5DMqeOAhts15beObR0bXp6ioxVuCvrsCJESF6iRzjGWUbX
s8Z/xk5pHfMngw27rDScTCNWXxe2yNkK+qY9XffuGuhWE3l/vvNFQ6WS4nhaO7PD
+mkdzIkredoAtieKWEiHFDcCgYBQetqcpoe3owSlslt/JWjFbKZiSVVB3qhWtqtt
mE4akjGDYBz+AKLMz3BighDUE5zkWo6VShzu8er1seOFbH+kzByF0vX37Sf0+rPi
bJ8QZfAzJYbQmhXVWh5MJxJO3d/x4KALfHjs1yERQkfpjhMonzu2t3cYnqIDl/Lv
QS4fMQKBgFx5masOJqHNx/HDOLtOvO8MeKbZgb2wzrBteJBs/KyFjP4fzZZseiVV
67XuwVxrLup7KzUaHK8PysA+ZgiT4ZlvyX+J+pFZA2XPtKTKCA3bKYtIG2JF5W1v
uHXl2FV53+kI2rF188v3jbuUhK0FrsUEXpN8C+dotMMLCLakbNXP
-----END RSA PRIVATE KEY-----`

// Cert for luci-token-server-test-1.fake.domain. Signed by 'Fake CA: fake.ca'.
const certWithCN = `-----BEGIN CERTIFICATE-----
MIIEFjCCAv6gAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwYDELMAkGA1UEBhMCVVMx
EzARBgNVBAgMCkNhbGlmb3JuaWExDTALBgNVBAcMBEJsYWgxEjAQBgNVBAoMCVN0
dWZmIEluYzEZMBcGA1UEAwwQRmFrZSBDQTogZmFrZS5jYTAeFw0xNjA0MDkwNDIx
MTJaFw0xNzA0MTkwNDIxMTJaMHQxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMQ0wCwYDVQQHDARCbGFoMRIwEAYDVQQKDAlTdHVmZiBJbmMxLTArBgNV
BAMMJGx1Y2ktdG9rZW4tc2VydmVyLXRlc3QtMS5mYWtlLmRvbWFpbjCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBAL3Or+NiOG2namAPiBfXTAfN/ENpdiH5
UqQPMUguTu16w8GtUWAqj6vjDiCGjFIKgTCc9DpjckJG7US6t0LuEOOFCG3asst+
K4+5HMP/bfyvcftp3j4cdLX3c/uS4+lIkUSOxUQovXLPpoEkKrgg2gVvCeZd5QmR
J3vH6Rzztx8ZfnZFF9GPE9mQrJ9fYZ1e2e8G3zdXVLkYoVw/lMDdmsaKmdBCdhjU
i2AM9xx5CnOhCzNS8W4jg/SccPMG/uVZ6SLkd5eNuhXLsfREBnsC2RU8Y0Uuw0Ty
//S9MRPskSfyNOFMib+FrPfH2Kf7wDPdtlrdtT/T8ur4J/l48s9gKMkCAwEAAaOB
xTCBwjAJBgNVHRMEAjAAMBEGCWCGSAGG+EIBAQQEAwIFoDAzBglghkgBhvhCAQ0E
JhYkT3BlblNTTCBHZW5lcmF0ZWQgQ2xpZW50IENlcnRpZmljYXRlMB0GA1UdDgQW
BBQf/Xtn7MQpybujv9/54DxdiNDKFzAfBgNVHSMEGDAWgBRhO7licgHsGIwDmWmP
zL+oymoPHjAOBgNVHQ8BAf8EBAMCBeAwHQYDVR0lBBYwFAYIKwYBBQUHAwIGCCsG
AQUFBwMEMA0GCSqGSIb3DQEBCwUAA4IBAQA9YXLIMwJbfQMMpTxPZLQoiqbG0fPB
xuSBGNYd/US6NIWLOg/v5tfN2GB+RAuB1Rz12eS+TmN7+A/lfNx0dFYwcfeOz05g
jQMwgUDmlnicMqENd0pswccS/mci215addFq6Wknti+To+TST0Ci5zmIt2fbBjmI
VRAWsPfLInwtW94S54UF38n2gp3iXizQLG2urSqotPsWIiyO+f2M3Q2ki3fDzimj
EyA+GFsGD6l0nQUySNyk2xE4S5CHOyLG0qWOsaJsEkTMnN+lrUh1bLUcI3bvVpVP
uwi+mmV6pbwEPKYNHpxHXSbEFnWwnZm1OtM28sP9O0D94XzRq2OfWiiD
-----END CERTIFICATE-----`

// Cert for CN=fuchsia-debian-dev-141242e1-us-central1-f-0psd with SAN
// DNS:fuchsia-debian-dev-141242e1-us-central1-f-0psd.c.fuchsia-infra.internal
// Signed by 'Fuchsia Infra CA'.
const certWithSAN = `
-----BEGIN CERTIFICATE-----
MIIFuTCCA6GgAwIBAgIJAJZJwMfxXGrnMA0GCSqGSIb3DQEBCwUAMBsxGTAXBgNV
BAMMEEZ1Y2hzaWEgSW5mcmEgQ0EwHhcNMTcwODE2MDEyNTEwWhcNMTcxMTE0MDEy
NTEwWjA5MTcwNQYDVQQDDC5mdWNoc2lhLWRlYmlhbi1kZXYtMTQxMjQyZTEtdXMt
Y2VudHJhbDEtZi0wcHNkMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA
z4pB9PB50ULz3rSDcksN4ZWJv0p5+DqKNxkBVcoqULNFDsD3I+zoOJn8EZgdNRD6
xAiFigQkZUOLgIYmDDmlWJdfPJM4Q9pWLPwq+ukqlWSA6WsoAJFLnzqjZSlQ3hKw
pyHgkQy3Y80Z4pxmlbKpDqpyiJpacoKGx0en8IYOf+dwu3d37b9jGftAbIDZqTdP
Cvfp5Z9m+LcDN/jFyL2cgvPDdrtskpKbIZy/80+Fh8MPLs/F327edEVEWv7cfvnP
RX0Y8tthdHNXEVDT/akzT2kRQBjiOGMhjNocau4po4+KU4lMBKvpWdg6ar0nZTBX
Rw4bRyYtIa71kSsJFCXO7+ljfyF8RVfZjb9CwNc1VWCzmPdQX7aJf0jgnbffU9oH
PAIJf9pSvFmrs3CyFz7QQGkLzvcLm5P2YDgG3IGRncyTTLuqkBtlkmGti1nM3iPP
rIyOeib1b7xl49AqsATFjk9GbfVHEVOx6EbpIWKi5I7fVTK8ax7kmE5heCUJ+nc2
HS+c/DaoGiPoly+7SuYTaFTeFaBKpZbS2JaqxwccHjLC02IgwoLFQrkaG4O+zZbG
HovdR+hQT1Bv1JYl7h7ztcyH+Xi0xwA0URLMqu+CGG17sHpYJLpus3OPXqGIeQiK
SWLMRF9EK99rO6fz/+8+DxYEJ2pmT/9fxLXGwKG/T9UCAwEAAaOB4TCB3jAMBgNV
HRMBAf8EAjAAMA4GA1UdDwEB/wQEAwIFIDAdBgNVHQ4EFgQU5NmocyuT3c+0v4S5
j8CUmRPzAqkwSwYDVR0jBEQwQoAUrQ+P1nNDqPCBSDtc4j6RkCgl/AqhH6QdMBsx
GTAXBgNVBAMMEEZ1Y2hzaWEgSW5mcmEgQ0GCCQCNC1KbQTIWhTBSBgNVHREESzBJ
gkdmdWNoc2lhLWRlYmlhbi1kZXYtMTQxMjQyZTEtdXMtY2VudHJhbDEtZi0wcHNk
LmMuZnVjaHNpYS1pbmZyYS5pbnRlcm5hbDANBgkqhkiG9w0BAQsFAAOCAgEAWFUw
2ncqnGKp4qmLmjS3E0C9CwAIciky2Vunb7Sb9AMnu5cCdiTLudQkoLwy80t0Y1TV
mEfGXn1Amt+B65X4TyRRSqFbDqLgUAb+0YX99f+mB9MKxaDDLPX8i3m3NLXd4che
OBRfeGY2kCE04svzA0t+Dy80jQENqu7a22tX5BFKSPTCEnNXXTH0X227vhwausTM
ngv10lsxNqxt0LimxB2gPjMms58fDEwUx1tj2k4BJmgfe2OW8lPqKXzXOOe8NI5k
5utCtd3aWdFRuJhpduUMdEQG920Cmb8PT6OeGrDdSV0nCmzG+fPy8O7sLzFlKsgQ
bX6YZX9f87k423gQZ7DP4Ic8t/1a30njZf+tBrABkr1kPDGajQjXK8MxtaTstn1A
jKeva9iI0QGECiwYfXKVJLDh9NYdD8QTzgMh2cWPNaPUJAvhe11gkH2+j6SE68YJ
ZtHVYstruzpnSdv/EjpcU7VvfOGBvjruksjCPkL09+EnH0hrw2BIOnEXA7gXhQV+
/qew6kPTNHlWNJHXXMrbZbWlBWjYZQcaqXcCBWHujMHy2P4RH9zMCiLE6uHHc3mL
q07s6UiAqamPwRd1A5OffPEvchkbKSaOOLPICpYu5Qg2LrZ0IAFS3r5y+5EXOJLV
3SsvIZgCBTBX8gzpcssCjvBiJSPUTTiowPE4+MA=
-----END CERTIFICATE-----`

var testingTime = time.Date(2015, time.February, 3, 4, 5, 6, 0, time.UTC)

var testingCA = certconfig.CA{
	CN: "Fake CA: fake.ca",
	ParsedConfig: &admin.CertificateAuthorityConfig{
		UniqueId: 123,
		KnownDomains: []*admin.DomainConfig{
			{
				Domain:               []string{"fake.domain"},
				MachineTokenLifetime: 3600,
			},
		},
	},
	AddedRev:   "cfg-added-rev",
	UpdatedRev: "cfg-updated-rev",
}

func testingContext(ca certconfig.CA) context.Context {
	ctx := gaetesting.TestingContext()
	ctx = info.GetTestable(ctx).SetRequestID("gae-request-id")
	ctx, _ = testclock.UseTime(ctx, testingTime)

	// Put mocked CA config in the datastore.
	ds.Put(ctx, &ca)
	certconfig.StoreCAUniqueIDToCNMap(ctx, map[int64]string{
		ca.ParsedConfig.UniqueId: ca.CN,
	})
	certconfig.UpdateCRLSet(ctx, ca.CN, certconfig.CRLShardCount, &pkix.CertificateList{})

	return ctx
}

func testingSigner() *signingtest.Signer {
	return signingtest.NewSigner(&signing.ServiceInfo{
		ServiceAccountName: "signer@testing.host",
		AppID:              "unit-tests",
		AppVersion:         "mocked-ver",
	})
}

// testingRawRequest is canned request for machine token before it is serialized
// and signed.
func testingRawRequest(ctx context.Context) *minter.MachineTokenRequest {
	return &minter.MachineTokenRequest{
		Certificate:        getTestCertDER(certWithCN),
		SignatureAlgorithm: minter.SignatureAlgorithm_SHA256_RSA_ALGO,
		IssuedAt:           google.NewTimestamp(clock.Now(ctx)),
		TokenType:          tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
	}
}

// testingMachineTokenRequest is canned request to MintMachineToken RPC.
func testingMachineTokenRequest(ctx context.Context) *minter.MintMachineTokenRequest {
	serialized, err := proto.Marshal(testingRawRequest(ctx))
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(serialized)
	signature, err := getTestPrivateKey().Sign(nil, digest[:], crypto.SHA256)
	if err != nil {
		panic(err)
	}
	return &minter.MintMachineTokenRequest{
		SerializedTokenRequest: serialized,
		Signature:              signature,
	}
}

func testingMachineToken(ctx context.Context, signer signing.Signer) string {
	_, tok, err := Mint(ctx, &MintParams{
		Cert:   getTestCert(certWithCN),
		Config: testingCA.ParsedConfig,
		Signer: signer,
	})
	if err != nil {
		panic(err)
	}
	return tok
}

func getTestPrivateKey() *rsa.PrivateKey {
	block, _ := pem.Decode([]byte(pkey))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return key
}

func getTestCert(cert string) *x509.Certificate {
	crt, err := x509.ParseCertificate(getTestCertDER(cert))
	if err != nil {
		panic(err)
	}
	return crt
}

func getTestCertDER(cert string) []byte {
	block, _ := pem.Decode([]byte(cert))
	return block.Bytes
}
