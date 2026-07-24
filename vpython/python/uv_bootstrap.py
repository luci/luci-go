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

"""Wrapper script to initialize and build a UV-backed virtualenv using UV."""

import argparse
import glob
import os
import subprocess
import sys
import shutil
import time
import urllib.request
import json
import re


DEFAULT_NETWORK_RETRY_TIMEOUT = 300  # Default to 5 minutes timeout


def get_network_retry_timeout():
    val = os.getenv("VPYTHON_NETWORK_RETRY_TIMEOUT")
    if val:
        try:
            return float(val)
        except ValueError:
            pass
    return DEFAULT_NETWORK_RETRY_TIMEOUT


NETWORK_ERROR_PATTERNS = [
    re.compile(p, re.IGNORECASE) for p in [
        r"Failed to establish a new connection",
        r"getaddrinfo failed",
        r"NewConnectionError",
        r"Max retries exceeded with url",
        r"NameResolutionError",
        r"Name or service not known",
        r"nodename nor servname provided",
        r"Temporary failure in name resolution",
        r"ConnectionRefusedError",
        r"Connection reset by peer",
        r"ConnectionResetError",
        r"ConnectTimeoutError",
        r"ReadTimeoutError",
        r"HTTPSConnectionPool",
        r"HTTPConnectionPool",
        r"Network is unreachable",
        r"No route to host",
        r"connection broken by",
        r"Retrying \(Retry\(total=",
        r"Failed to fetch",
        r"Failed to download",
        r"failed to lookup address",
        r"gai error",
        r"dns error",
        r"operation timed out",
        r"connection timed out",
        r"connection refused",
        r"connection reset",
        r"error sending request",
        r"error connecting to",
        r"http connection error",
        r"network error",
    ]
]


def is_network_error(output_str):
    if not output_str:
        return False
    return any(p.search(output_str) for p in NETWORK_ERROR_PATTERNS)


def report_to_endpoint(package, version, context, report_url):
    data = {
        "package": package,
        "version": version,
        "context": f"vpython-{context}"
    }
    req = urllib.request.Request(
        report_url,
        data=json.dumps(data).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST"
    )
    try:
        with urllib.request.urlopen(req, timeout=2) as response:
            pass
    except Exception:
        # Ignore all errors to ensure it's non-blocking
        pass


def try_report_missing_uv(stderr, report_url):
    stderr = " ".join(stderr.split())
    # Example: "no versions of foo were found"
    match = re.search(r"no versions of ([\w.-]+) were found", stderr)
    if match:
        package = match.group(1)
        report_to_endpoint(package, "", "uv", report_url)
        return

    # Example: "No compatible versions were found in ...: foo==1.2.3"
    match = re.search(r"No compatible versions were found in [^:]+: ([\w.-]+)==([\w.-]+)", stderr)
    if match:
        package = match.group(1)
        version = match.group(2)
        report_to_endpoint(package, version, "uv", report_url)
        return

    # Example: "Because foo was not found in the package registry and you require foo==1.0.0"
    match = re.search(r"Because ([\w.-]+) was not found in the package registry and you require \1==([\w.-]+)", stderr)
    if match:
        package = match.group(1)
        version = match.group(2)
        report_to_endpoint(package, version, "uv", report_url)
        return

    # More general fallback
    match = re.search(r"Because ([\w.-]+) was not found in the package registry", stderr)
    if match:
        package = match.group(1)
        report_to_endpoint(package, "", "uv", report_url)
        return

    print("vpython: Failed to parse missing package from uv error", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--target-dir", required=False, help="Target virtualenv directory destination path")
    parser.add_argument("--uv-bin", required=True, help="Absolute path to the pre-packaged uv binary")
    parser.add_argument("--python-bin", required=True, help="Absolute path to the host python interpreter binary")
    parser.add_argument("--req-file", required=True, help="Absolute path to standard requirements.txt file")
    args = parser.parse_args()

    target_dir = args.target_dir or os.getenv("out")
    if not target_dir:
        print("Error: either --target-dir parameter or 'out' environment variable is mandatory", file=sys.stderr)
        return 1

    # Create virtual environment.
    venv_cmd = [
        args.uv_bin,
        "venv",
        target_dir,
        "--python",
        args.python_bin,
    ]
    env = os.environ.copy()
    env["UV_PYTHON_DOWNLOADS"] = "never"
    env["UV_NO_BUILD"] = "true"
    env["UV_KEYRING_PROVIDER"] = "disabled"
    env["UV_NO_CACHE"] = "true"

    # Configure package registry mirror.
    ar_url = os.getenv("VPYTHON_AR_URL")
    if not ar_url:
        raise RuntimeError("VPYTHON_AR_URL environment variable is mandatory")
    env["UV_DEFAULT_INDEX"] = ar_url

    print("Creating virtualenv via UV...")
    subprocess.check_call(venv_cmd, env=env)

    # Install requirements.
    venv_python = os.path.join(target_dir, "bin", "python3")
    if sys.platform == "win32":
        venv_python = os.path.join(target_dir, "Scripts", "python.exe")

    sync_cmd = [
        args.uv_bin,
        "pip",
        "install",
        "--python",
        venv_python,
        "--no-build",
        "-r",
        args.req_file,
    ]

    print("Installing spec requirements via UV...")
    attempt = 0
    start_time = time.time()
    max_timeout = get_network_retry_timeout()
    while True:
        try:
            res = subprocess.run(sync_cmd, env=env, capture_output=True, text=True, check=True)
            if res.stdout:
                print(res.stdout)
            break
        except subprocess.CalledProcessError as e:
            output_str = (e.stderr or "") + "\n" + (e.stdout or "")
            if is_network_error(output_str):
                elapsed = time.time() - start_time
                if elapsed >= max_timeout:
                    print(
                        f"\nvpython UV INSTALL failed due to network error and exceeded maximum retry timeout ({elapsed:.1f}s >= {max_timeout:.1f}s). Giving up.",
                        file=sys.stderr,
                        flush=True
                    )
                else:
                    attempt += 1
                    print(
                        f"\nvpython UV INSTALL failed due to network error (attempt {attempt}, elapsed {elapsed:.1f}s / {max_timeout:.1f}s). Retrying in 5 seconds...",
                        file=sys.stderr,
                        flush=True
                    )
                    if e.stdout:
                        print(e.stdout, flush=True)
                    if e.stderr:
                        print(e.stderr, file=sys.stderr, flush=True)
                    time.sleep(5)
                    continue

            if e.stdout:
                print(e.stdout)
            if e.stderr:
                print(e.stderr, file=sys.stderr)
            report_url = os.getenv("VPYTHON_REPORT_MISSING_URL")
            if report_url:
                try_report_missing_uv(e.stderr, report_url)
            sys.exit(e.returncode)

    # Replicate python.exe to python3.exe in the venv from current executable.
    # Replace venv launchers with actual physical copies of host CPython binary and
    # copy DLLs so that Python can be executed entirely from Venv.
    if sys.platform == "win32":
        scripts_dir = os.path.join(target_dir, "Scripts")
        cpython_dir = os.path.dirname(sys.executable)
        p1 = os.path.join(scripts_dir, "python.exe")
        p3 = os.path.join(scripts_dir, "python3.exe")
        for p in (p1, p3):
            if os.path.exists(p):
                os.remove(p)
            shutil.copyfile(sys.executable, p)
        for dll in glob.glob(os.path.join(cpython_dir, '*.dll')):
            target_dll = os.path.join(scripts_dir, os.path.basename(dll))
            if os.path.exists(target_dll):
                os.remove(target_dll)
            shutil.copyfile(dll, target_dll)

    print("UV Virtualenv synchronization complete successfully!")


if __name__ == "__main__":
    sys.exit(main())
