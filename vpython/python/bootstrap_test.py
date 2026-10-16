# Copyright 2026 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import unittest
from unittest import mock
from bootstrap import is_network_error as pip_is_network_error, get_network_retry_timeout as pip_get_timeout
from uv_bootstrap import is_network_error as uv_is_network_error, get_network_retry_timeout as uv_get_timeout


class TestNetworkErrorDetection(unittest.TestCase):

    def test_user_reported_pip_error(self):
        user_log = """
============================================================
vpython AR INSTALL FAILED
Looking in indexes: https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/

WARNING: Retrying (Retry(total=4, connect=None, read=None, redirect=None, status=None)) after connection broken by 'NewConnectionError('<pip._vendor.urllib3.connection.HTTPSConnection object at 0x000001ECDCE13890>: Failed to establish a new connection: [Errno 11001] getaddrinfo failed')': /chrome-python-ar/chrome-python-ar/simple/pywin32/

WARNING: Retrying (Retry(total=3, connect=None, read=None, redirect=None, status=None)) after connection broken by 'NewConnectionError('<pip._vendor.urllib3.connection.HTTPSConnection object at 0x000001ECDCDA5110>: Failed to establish a new connection: [Errno 11001] getaddrinfo failed')': /chrome-python-ar/chrome-python-ar/simple/pywin32/

ERROR: Could not find a version that satisfies the requirement pywin32==306 (from versions: none)
ERROR: No matching distribution found for pywin32==306
============================================================
"""
        self.assertTrue(pip_is_network_error(user_log))
        self.assertTrue(uv_is_network_error(user_log))

    def test_common_network_errors(self):
        errors = [
            "Failed to establish a new connection: [Errno 11001] getaddrinfo failed",
            "Max retries exceeded with url /simple/foo",
            "NameResolutionError: Failed to resolve host",
            "curl: (6) Could not resolve host: us-python.pkg.dev; Name or service not known",
            "ConnectionRefusedError: [Errno 111] Connection refused",
            "ConnectionResetError: [Errno 104] Connection reset by peer",
            "ReadTimeoutError: HTTPSConnectionPool(host='us-python.pkg.dev', port=443): Read timed out.",
            "error: Failed to download distributions: failed to lookup address information",
            "gai error: temporary failure in name resolution",
            "http connection error: operation timed out",
            "HTTPSConnectionPool(host='us-python.pkg.dev', port=443): Max retries exceeded with url: /simple/foo/ (Caused by SSLError(SSLEOFError(8, 'EOF occurred in violation of protocol (_ssl.c:1129)')))",
        ]
        for err in errors:
            self.assertTrue(pip_is_network_error(err), msg=f"Failed for pip: {err}")
            self.assertTrue(uv_is_network_error(err), msg=f"Failed for uv: {err}")

    def test_non_network_errors(self):
        errors = [
            "ERROR: Could not find a version that satisfies the requirement foo==1.0.0 (from versions: none)\nERROR: No matching distribution found for foo==1.0.0",
            "Because foo was not found in the package registry and you require foo==1.0.0",
            "No compatible versions were found in index: bar==2.0.0",
            "SyntaxError: invalid syntax in setup.py",
            "OSError: Could not find a suitable TLS CA certificate bundle, invalid path: /tmp/missing-ca.crt",
        ]
        for err in errors:
            self.assertFalse(pip_is_network_error(err), msg=f"Should be False for pip: {err}")
            self.assertFalse(uv_is_network_error(err), msg=f"Should be False for uv: {err}")

    def test_proxy_and_tls_errors_gated_on_env(self):
        proxy_or_tls_errors = [
            "HTTPSConnectionPool(host='us-python.pkg.dev', port=443): Max retries exceeded with url: /simple/foo/ (Caused by ProxyError('Cannot connect to proxy.', OSError('Tunnel connection failed: 403 Forbidden')))",
            "error: Failed to fetch: `https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/six/`\n  Caused by: HTTP status client error (403 Forbidden) for url (https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/six/)",
            "error: Failed to download `six==1.16.0`\n  Caused by: Request to GET /chrome-python-ar/simple/six/ not allowed by policy",
            "HTTPSConnectionPool(host='us-python.pkg.dev', port=443): Max retries exceeded with url: /simple/foo/ (Caused by SSLError(SSLCertVerificationError(1, '[SSL: CERTIFICATE_VERIFY_FAILED] certificate verify failed: self signed certificate in certificate chain')))",
            "error: Failed to fetch: `https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/six/`\n  Caused by: invalid peer certificate: UnknownIssuer",
        ]

        with mock.patch.dict(os.environ, {}, clear=True):
            for err in proxy_or_tls_errors:
                self.assertTrue(pip_is_network_error(err), msg=f"Should retry without proxy/TLS env for pip: {err}")
                self.assertTrue(uv_is_network_error(err), msg=f"Should retry without proxy/TLS env for uv: {err}")

        with mock.patch.dict(os.environ, {"HTTPS_PROXY": "http://127.0.0.1:8080"}, clear=True):
            for err in proxy_or_tls_errors:
                self.assertFalse(pip_is_network_error(err), msg=f"Should fail fast with HTTPS_PROXY for pip: {err}")
                self.assertFalse(uv_is_network_error(err), msg=f"Should fail fast with HTTPS_PROXY for uv: {err}")

            transient_400 = (
                "error: Failed to fetch: `https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/six/`\n"
                "  Caused by: HTTP status client error (400 Bad Request) for url (https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/six/)"
            )
            self.assertTrue(pip_is_network_error(transient_400))
            self.assertTrue(uv_is_network_error(transient_400))

    def test_network_retry_timeout_config(self):
        self.assertEqual(pip_get_timeout(), 300)
        self.assertEqual(uv_get_timeout(), 300)

        os.environ["VPYTHON_NETWORK_RETRY_TIMEOUT"] = "60"
        try:
            self.assertEqual(pip_get_timeout(), 60)
            self.assertEqual(uv_get_timeout(), 60)
        finally:
            del os.environ["VPYTHON_NETWORK_RETRY_TIMEOUT"]


if __name__ == "__main__":
    unittest.main()
