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
import os
import subprocess
import sys
import tempfile

DISABLE_PERIODIC_UPDATE = False  # This option only exists for newer virtualenv
if sys.version_info[0] > 2:
  ISOLATION_FLAG = '-I'
  COMPILE_WORKERS = min(os.cpu_count() or 1, 4) # Limit the concurrency to 4
  if sys.version_info[1] >= 11:
    DISABLE_PERIODIC_UPDATE = True
else:
  ISOLATION_FLAG = '-sSE'
  COMPILE_WORKERS = 1

# Create virtual environment in ${out} directory
virtualenv = glob.glob(
    os.path.join(os.environ['virtualenv'], '*', 'virtualenv.py*'))[0]

virtualenv_flags = ['--no-download', '--always-copy']
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
  def _info(msg):
    try:
      with open(os.path.join(os.environ['out'], '.vpython_bootstrap_info.txt'), 'a') as f:
        f.write(msg + '\n')
    except Exception:
      pass

  pip = glob.glob(os.path.join(os.environ['out'], '*', 'pip*'))[0]
  try:
    _info('Attempting to install wheels from Artifact Registry...')
    ar_url = os.environ.get('VPYTHON_AR_URL', 'https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/')
    command = [
        pip,
        'install',
        '--isolated',
        '--compile',
        '--index-url',
        ar_url,
        '--requirement',
        os.path.join(os.environ['wheels'], 'requirements.txt'),
    ]
    env = os.environ.copy()
    # Drop possible NETRC to avoid uncomfortable situation where we try to
    # apply credentials where we don't need them.
    env['NETRC'] = os.devnull
    output = subprocess.check_output(command, env=env, stderr=subprocess.STDOUT)
    if hasattr(output, 'decode'):
      output = output.decode('utf-8', 'replace')
    print(output)
    _info('Installed wheels successfully from Artifact Registry.')
  except subprocess.CalledProcessError as e:
    output = e.output
    if hasattr(output, 'decode'):
      output = output.decode('utf-8', 'replace')
    _info('\n' + '=' * 60)
    _info('Artifact Registry install failed. Falling back to CIPD.')
    _info('Please report this issue at https://crbug.com/492362903')
    _info('and copy-paste the following pip output:')
    _info('-' * 60)
    _info(output.strip())
    _info('=' * 60 + '\n')

    wheels_root = os.environ['wheels']
    wheels_dir = os.path.join(wheels_root, 'wheels')
    if not os.path.isdir(wheels_dir):
      _info('CIPD wheels missing from store. Falling back to on-demand fetch...')
      ensure_file = os.path.join(wheels_root, 'ensure.txt')
      # Export wheels to a local directory in the output.
      local_wheels = os.path.join(os.environ['out'], 'on_demand_wheels')
      # ref: common/common.go
      cipd_path = os.environ.get('VPYTHON_CIPD_PATH', 'cipd')
      cipd_cmd = [cipd_path]
      if sys.platform == 'win32' and cipd_cmd[0].lower().endswith('.bat'):
        cipd_cmd = ['cmd.exe', '/C'] + cipd_cmd
      subprocess.check_call(cipd_cmd + [
          'export', '-ensure-file',
          ensure_file, '-root', local_wheels
      ])
      wheels_dir = os.path.join(local_wheels, 'wheels')

    wheels = sorted(glob.glob(os.path.join(wheels_dir, '*.whl')))
    subprocess.check_call([
        pip,
        'install',
        '--isolated',
        '--compile',
        '--no-index',
        '--find-links',
        wheels_dir,
    ] + wheels)

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
