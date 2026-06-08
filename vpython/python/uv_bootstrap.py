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
import os
import subprocess
import sys


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

    # Configure package registry mirror.
    ar_url = os.getenv("VPYTHON_AR_URL")
    if not ar_url:
        raise RuntimeError("VPYTHON_AR_URL environment variable is mandatory")
    env["UV_DEFAULT_INDEX"] = ar_url

    print("Creating virtualenv via UV...")
    subprocess.check_call(venv_cmd, env=env)

    # Install requirements.
    venv_python = os.path.join(target_dir, "bin", "python")
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
    subprocess.check_call(sync_cmd, env=env)

    print("UV Virtualenv synchronization complete successfully!")


if __name__ == "__main__":
    sys.exit(main())
