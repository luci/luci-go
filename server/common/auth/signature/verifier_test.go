// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature_test

import (
	"encoding/base64"
	"testing"

	"github.com/luci/luci-go/server/common/auth/signature"
)

func TestCheckRuns(t *testing.T) {
	encodedSig := "KCdriOOYuQeTa0KJYdsfaLrbn6LjaJlLJRpWO25Xif+3G8hvrpCntFkNsH/n99hW8tDv6+reZDQDFsAxPx1tS7MeCKDAAlbEox9SkejcPAq+o9w9ql0BY6hukrz2oNmXIoywZdl2dxEqYV02CvpU4S3ZsI+QSlsju49Bddid7q4Dg5jD63RI2FZE1UEGxDPIeF2W2aMtzKMi2TZfTr5HGXEOeTutzB16oFgUlRYikH0KlufiwbrmgyIs5ltU5bgu5flJX5+rgq0ofW/M3OeqWu4eCrWnY7SMxTpkTq+JbYMn41I7WiJvRBU/Kbm+EEugHgOhfYTYCnr9tWQEXzDylw=="
	encodedBlob := "CiIKFWNocm9tZS1pbmZyYS1hdXRoLWRldhAJGI+qwor9pMYCEr0DCgASACKVAQoOYWRtaW5pc3RyYXRvcnMSFXVzZXI6dGVzdEBleGFtcGxlLmNvbSocVXNlcnMgdGhhdCBjYW4gbWFuYWdlIGdyb3VwczDh07Gz+KLGAjodc2VydmljZTpjaHJvbWUtaW5mcmEtYXV0aC1kZXZA19Oxs/iixgJKHXNlcnZpY2U6Y2hyb21lLWluZnJhLWF1dGgtZGV2IqoBCgR0ZXN0Ehd1c2VyOmV4YW1wbGVAZ29vZ2xlLmNvbRIZdXNlcjpleGFtcGxlMkBleGFtcGxlLmNvbRIZdXNlcjpleGFtcGxlM0BleGFtcGxlLm5ldCoTVGhpcyBpcyB0ZXN0IGdyb3VwLjCnhria0aTGAjoVdXNlcjp0ZXN0QGV4YW1wbGUuY29tQJyGuJrRpMYCShV1c2VyOnRlc3RAZXhhbXBsZS5jb20icgoFdGVzdDISFXVzZXI6bmVzdEBleGFtcGxlLmNvbSIEdGVzdCoMbmVzdGVkIGdyb3VwMK6N0aTRpMYCOhV1c2VyOnRlc3RAZXhhbXBsZS5jb21An43RpNGkxgJKFXVzZXI6dGVzdEBleGFtcGxlLmNvbRoFMS4xLjI="
	pem := "\n-----BEGIN CERTIFICATE-----\nMIIC/jCCAeagAwIBAgIIQTBFcRw3moMwDQYJKoZIhvcNAQEFBQAwIjEgMB4GA1UE\nAxMXcm9ib3RqYXZhLmEuYXBwc3BvdC5jb20wHhcNMTEwMjIzMTUwNzQ5WhcNMTEw\nMjI0MTYwNzQ5WjAiMSAwHgYDVQQDExdyb2JvdGphdmEuYS5hcHBzcG90LmNvbTCC\nASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAJd0YJCQWvQMa+7L/orCt3D0\nhVtkdAkeGSikuT4U7mNrxBuOaAbxCIGhRbUe2p+uvRF6MZtLvoU1h9qEFo/wAVDO\nHN4WHhw3VLl/OVuredRfe8bBTi0KqdgUBrKr8V61n26N3B4Ma9dkTMbcODC/XCfP\nIRJnTIf4Z1vnoEfWQEJDfW9QLJFyJF17hpp9l5S1uuMJBxjYMsZ3ExLqSFhM7IbN\n1PDBAb6zGtI7b9AVP+gxS1hjXiJoZA32IWINAZiPV+0k925ecsV0BkI0zV4Ta06F\nJexNx040y5ivr4C214GRUM3UKihirTcEOBS1a7SRi5wCPh/wT0A8gN6NNbTNjc0C\nAwEAAaM4MDYwDAYDVR0TAQH/BAIwADAOBgNVHQ8BAf8EBAMCB4AwFgYDVR0lAQH/\nBAwwCgYIKwYBBQUHAwIwDQYJKoZIhvcNAQEFBQADggEBAD+h2D+XGIHWMwPCA2DN\nJgMhN1yTTJ8dtwbiQIhfy8xjOJbrzZaSEX8g2gDm50qaEl5TYHHr2zvAI1UMWdR4\nnx9TN7I9u3GoOcQsmn9TaOKkBDpMv8sPtFBal3AR5PwR5Sq8/4L/M22LX/TN0eIF\nY4LnkW+X/h442N8a1oXn05UYtFo+p/6emZb1S84WZAnONGtF5D1Z6HuX4ikDI5m+\niZbwm47mLkV8yuTZGKI1gJsWmAsElPkoWVy2X0t69ecBOYyn3wMmQhkLk2+7lLlD\n/c4kygP/941fe1Wb/T9yGeBXFwEvJ4jWbX93Q4Xhk9UgHlso9xkCu9QeWFvJqufR\n5Cc=\n-----END CERTIFICATE-----\n"

	sig, err := base64.StdEncoding.DecodeString(encodedSig)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := base64.StdEncoding.DecodeString(encodedBlob)
	if err != nil {
		t.Fatal(err)
	}

	err = signature.Check(blob, []byte(pem), sig)
	if err != nil {
		t.Errorf("Check(%q, %q, %q) = %v; want <nil>", blob, pem, sig, err)
	}
}

func TestCheckShouldFailWithWrongSignature(t *testing.T) {
	encodedBlob := "CiIKFWNocm9tZS1pbmZyYS1hdXRoLWRldhAJGI+qwor9pMYCEr0DCgASACKVAQoOYWRtaW5pc3RyYXRvcnMSFXVzZXI6dGVzdEBleGFtcGxlLmNvbSocVXNlcnMgdGhhdCBjYW4gbWFuYWdlIGdyb3VwczDh07Gz+KLGAjodc2VydmljZTpjaHJvbWUtaW5mcmEtYXV0aC1kZXZA19Oxs/iixgJKHXNlcnZpY2U6Y2hyb21lLWluZnJhLWF1dGgtZGV2IqoBCgR0ZXN0Ehd1c2VyOmV4YW1wbGVAZ29vZ2xlLmNvbRIZdXNlcjpleGFtcGxlMkBleGFtcGxlLmNvbRIZdXNlcjpleGFtcGxlM0BleGFtcGxlLm5ldCoTVGhpcyBpcyB0ZXN0IGdyb3VwLjCnhria0aTGAjoVdXNlcjp0ZXN0QGV4YW1wbGUuY29tQJyGuJrRpMYCShV1c2VyOnRlc3RAZXhhbXBsZS5jb20icgoFdGVzdDISFXVzZXI6bmVzdEBleGFtcGxlLmNvbSIEdGVzdCoMbmVzdGVkIGdyb3VwMK6N0aTRpMYCOhV1c2VyOnRlc3RAZXhhbXBsZS5jb21An43RpNGkxgJKFXVzZXI6dGVzdEBleGFtcGxlLmNvbRoFMS4xLjI="
	pem := "\n-----BEGIN CERTIFICATE-----\nMIIC/jCCAeagAwIBAgIIQTBFcRw3moMwDQYJKoZIhvcNAQEFBQAwIjEgMB4GA1UE\nAxMXcm9ib3RqYXZhLmEuYXBwc3BvdC5jb20wHhcNMTEwMjIzMTUwNzQ5WhcNMTEw\nMjI0MTYwNzQ5WjAiMSAwHgYDVQQDExdyb2JvdGphdmEuYS5hcHBzcG90LmNvbTCC\nASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAJd0YJCQWvQMa+7L/orCt3D0\nhVtkdAkeGSikuT4U7mNrxBuOaAbxCIGhRbUe2p+uvRF6MZtLvoU1h9qEFo/wAVDO\nHN4WHhw3VLl/OVuredRfe8bBTi0KqdgUBrKr8V61n26N3B4Ma9dkTMbcODC/XCfP\nIRJnTIf4Z1vnoEfWQEJDfW9QLJFyJF17hpp9l5S1uuMJBxjYMsZ3ExLqSFhM7IbN\n1PDBAb6zGtI7b9AVP+gxS1hjXiJoZA32IWINAZiPV+0k925ecsV0BkI0zV4Ta06F\nJexNx040y5ivr4C214GRUM3UKihirTcEOBS1a7SRi5wCPh/wT0A8gN6NNbTNjc0C\nAwEAAaM4MDYwDAYDVR0TAQH/BAIwADAOBgNVHQ8BAf8EBAMCB4AwFgYDVR0lAQH/\nBAwwCgYIKwYBBQUHAwIwDQYJKoZIhvcNAQEFBQADggEBAD+h2D+XGIHWMwPCA2DN\nJgMhN1yTTJ8dtwbiQIhfy8xjOJbrzZaSEX8g2gDm50qaEl5TYHHr2zvAI1UMWdR4\nnx9TN7I9u3GoOcQsmn9TaOKkBDpMv8sPtFBal3AR5PwR5Sq8/4L/M22LX/TN0eIF\nY4LnkW+X/h442N8a1oXn05UYtFo+p/6emZb1S84WZAnONGtF5D1Z6HuX4ikDI5m+\niZbwm47mLkV8yuTZGKI1gJsWmAsElPkoWVy2X0t69ecBOYyn3wMmQhkLk2+7lLlD\n/c4kygP/941fe1Wb/T9yGeBXFwEvJ4jWbX93Q4Xhk9UgHlso9xkCu9QeWFvJqufR\n5Cc=\n-----END CERTIFICATE-----\n"

	blob, err := base64.StdEncoding.DecodeString(encodedBlob)
	if err != nil {
		t.Fatal(err)
	}

	sig := []byte("wrong")
	err = signature.Check(blob, []byte(pem), sig)
	if err == nil {
		t.Errorf("Check(%q, %q, %q) = <nil>; want err", blob, pem, sig)
	}
}
