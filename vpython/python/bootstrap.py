# Copyright 2022 The LUCI Authors.
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

import glob
import json
import os
import re
import time
try:
    from urllib.request import Request, urlopen
except ImportError:
    from urllib2 import Request, urlopen
import subprocess
import sys
import tempfile
import shutil

DISABLE_PERIODIC_UPDATE = False  # This option only exists for newer virtualenv
if sys.version_info[0] > 2:
  ISOLATION_FLAG = '-I'
  COMPILE_WORKERS = min(os.cpu_count() or 1, 4) # Limit the concurrency to 4
  if sys.version_info[1] >= 11:
    DISABLE_PERIODIC_UPDATE = True
else:
  ISOLATION_FLAG = '-sSE'
  COMPILE_WORKERS = 1

DEFAULT_NETWORK_RETRY_TIMEOUT = 300  # Default to 5 minutes timeout

def get_network_retry_timeout():
    val = os.environ.get("VPYTHON_NETWORK_RETRY_TIMEOUT")
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
        "context": "vpython-" + context
    }
    payload = json.dumps(data)
    if sys.version_info[0] > 2:
        payload = payload.encode("utf-8")
    req = Request(
        report_url,
        data=payload,
        headers={"Content-Type": "application/json"}
    )
    try:
        res = urlopen(req, timeout=2)
        res.close()
    except Exception:
        # Ignore all errors to ensure it's non-blocking
        pass

def try_report_missing_pip(output, report_url):
    output = " ".join(output.split())
    match = re.search(r"Could not find a version that satisfies the requirement ([\w.-]+)==([\w.-]+)", output)
    if not match:
        match = re.search(r"No matching distribution found for ([\w.-]+)==([\w.-]+)", output)

    if match:
        package = match.group(1)
        version = match.group(2)
        report_to_endpoint(package, version, "pip", report_url)
    else:
        match = re.search(r"No matching distribution found for ([\w.-]+)", output)
        if match:
            package = match.group(1)
            report_to_endpoint(package, "", "pip", report_url)

def main():
  # Create virtual environment in ${out} directory
  virtualenv = glob.glob(
      os.path.join(os.environ['virtualenv'], '*', 'virtualenv.py*'))[0]

  virtualenv_flags = ['--no-download', '--always-copy', '--clear']
  if DISABLE_PERIODIC_UPDATE:
    virtualenv_flags.append('--no-periodic-update')

  args = [
      sys.executable, '-B', ISOLATION_FLAG,
      # Use realpath to mitigate https://github.com/pypa/virtualenv/issues/1949
      os.path.realpath(virtualenv)
  ] + virtualenv_flags + [os.environ['out']]
  if virtualenv.endswith('.pyz'):
    # .pyz ensures Python3 so we can use TemporaryDirectory.
    with tempfile.TemporaryDirectory() as d:
      subprocess.check_call(args + ['--app-data', d])
  else:
    subprocess.check_call(args)

  # Install wheels to virtual environment
  if 'wheels' in os.environ:
    pip = glob.glob(os.path.join(os.environ['out'], '*', 'pip*'))[0]
    requirements_file = os.path.join(os.environ['wheels'], 'requirements.txt')

    ar_url = os.environ.get('VPYTHON_AR_URL')
    if not ar_url:
      raise RuntimeError("VPYTHON_AR_URL environment variable is mandatory")

    command = [
        pip, 'install',
        '--disable-pip-version-check', '--isolated',
        '--no-cache-dir',
        '--compile',
        '--only-binary=:all:',
        '--index-url', ar_url,
        '--requirement', requirements_file
    ]
    target_arch_file = os.path.join(os.environ['wheels'], 'target_arch.txt')
    if os.path.exists(target_arch_file):
      with open(target_arch_file) as f:
        target_arch = f.read().strip()
      if target_arch == 'x86_64' and sys.platform == 'darwin':
        # Force x86_64 via arch -x86_64 for pip!
        command = ['arch', '-x86_64'] + command
    env = os.environ.copy()
    env['NETRC'] = os.devnull
    env['PIP_KEYRING_PROVIDER'] = 'disabled'

    attempt = 0
    start_time = time.time()
    max_timeout = get_network_retry_timeout()
    while True:
      try:
        subprocess.check_output(command, env=env, stderr=subprocess.STDOUT)
        break
      except subprocess.CalledProcessError as e:
        output_str = e.output.decode('utf-8', errors='ignore')
        if is_network_error(output_str):
          elapsed = time.time() - start_time
          if elapsed >= max_timeout:
            sys.stderr.write(
                "\nvpython AR INSTALL failed due to network error and exceeded maximum retry timeout (%.1fs >= %.1fs). Giving up.\n"
                % (elapsed, max_timeout)
            )
            sys.stderr.write(output_str + "\n")
            sys.stderr.flush()
          else:
            attempt += 1
            sys.stderr.write(
                "\nvpython AR INSTALL failed due to network error (attempt %d, elapsed %.1fs / %.1fs). Retrying in 5 seconds...\n"
                % (attempt, elapsed, max_timeout)
            )
            sys.stderr.write(output_str + "\n")
            sys.stderr.flush()
            time.sleep(5)
            continue

        sys.stderr.write('\n' + '=' * 60 + '\n')
        sys.stderr.write("vpython AR INSTALL FAILED\n")
        sys.stderr.write(output_str + "\n")
        if '429' in output_str and 'Too Many Requests' in output_str:
          sys.stderr.write("AR likely got overflown with request. Please retry after some time.\n")
        else:
          sys.stderr.write("If you see this, it means some python packages are missing from Artifact Registry or installation failed.\n")
          sys.stderr.write("Please report this issue at https://crbug.com/492362903\n")
        sys.stderr.write('=' * 60 + '\n')
        report_url = os.environ.get("VPYTHON_REPORT_MISSING_URL")
        if report_url:
          try_report_missing_pip(output_str, report_url)
        raise

  # Generate all .pyc in the output directory. This prevent generating .pyc on the
  # fly, which modifies derivation output after the build.
  # It may fail because lack of permission. Ignore the error since it won't affect
  # correctness if .pyc can't be written to the directory anyway.
  try:
    subprocess.check_call([
        sys.executable, ISOLATION_FLAG, '-m', 'compileall', '-j',
        str(COMPILE_WORKERS), os.environ['out']
    ])
  except subprocess.CalledProcessError as e:
    print('complieall failed and ignored: {}'.format(e.returncode))

  # Replicate python.exe to python3.exe in the venv.
  # Replace venv launchers with actual physical copies of host CPython binary and
  # copy DLLs so that Python can be executed entirely from Venv.
  if sys.platform == 'win32':
    scripts_dir = os.path.join(os.environ['out'], 'Scripts')
    cpython_dir = os.path.dirname(sys.executable)
    p1 = os.path.join(scripts_dir, 'python.exe')
    p3 = os.path.join(scripts_dir, 'python3.exe')
    for p in (p1, p3):
      if os.path.exists(p):
        os.remove(p)
      shutil.copyfile(sys.executable, p)
    for dll in glob.glob(os.path.join(cpython_dir, '*.dll')):
      target_dll = os.path.join(scripts_dir, os.path.basename(dll))
      if os.path.exists(target_dll):
          os.remove(target_dll)
      shutil.copyfile(dll, target_dll)


if __name__ == '__main__':
  main()
