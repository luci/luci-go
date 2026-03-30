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
import subprocess
import sys
import tempfile
import contextlib

@contextlib.contextmanager
def temporary_directory():
  # Python 2.7 compatibility: tempfile.TemporaryDirectory was introduced in 3.2.
  name = tempfile.mkdtemp()
  try:
    yield name
  finally:
    import shutil
    shutil.rmtree(name, ignore_errors=True)

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
  requirements_file = os.path.join(os.environ['wheels'], 'requirements.txt')
  requirements = []
  with open(requirements_file, 'r') as f:
    for line in f:
      line = line.strip()
      if line and not line.startswith('#'):
        requirements.append(line)

  failed_requirements = []
  ar_url = os.environ.get('VPYTHON_AR_URL', 'https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/')

  with temporary_directory() as wheels_cache:
    wheels_dir = os.path.join(wheels_cache, 'wheels')
    if not os.path.exists(wheels_dir):
      os.makedirs(wheels_dir)

    _info('Attempting to download wheels itemized from Artifact Registry...')
    for req in requirements:
      try:
        _info('Downloading %s from AR...' % req)
        command = [
            pip, 'download',
            '--disable-pip-version-check', '--isolated',
            '--no-deps', '--dest', wheels_dir, '--index-url', ar_url, req
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
        subprocess.check_output(command, env=env, stderr=subprocess.STDOUT)
      except subprocess.CalledProcessError as e:
        _info('Failed to download %s from AR.' % req)
        failed_requirements.append(req)

    if failed_requirements:
      _info('Falling back to CIPD for %d requirements.' % len(failed_requirements))
      wheels_root = os.environ['wheels']
      existing_wheels_dir = os.path.join(wheels_root, 'wheels')

      if os.path.isdir(existing_wheels_dir):
        # If CIPD wheels exist, we can use them.
        _info('CIPD wheels are already cached. Will use them as fallback.')
      else:
        # If CIPD wheels are not cached, download only the failed ones on-demand
        _info('CIPD wheels missing from store. Falling back to on-demand fetch...')
        ensure_file = os.path.join(wheels_root, 'ensure.txt')

        mapping_file = os.path.join(wheels_root, 'mapping.json')
        mapping = {}
        if os.path.exists(mapping_file):
          with open(mapping_file, 'r') as f:
            mapping = json.load(f)

        ensure_file_to_use = ensure_file
        if mapping:
          _info('Filtering ensure.txt using mapping.json to only download failed wheels...')
          with open(ensure_file, 'r') as f:
            lines = f.readlines()

          filtered_lines = []
          for line in lines:
            stripped = line.strip()
            if not stripped or stripped.startswith('#') or stripped.startswith('$') or stripped.startswith('@'):
              filtered_lines.append(line)
              continue

            parts = stripped.split()
            if not parts:
              continue
            pkg_template = parts[0]

            matched = False
            for failed in failed_requirements:
              pip_name = failed.split('==')[0].lower()
              if pip_name in mapping and mapping[pip_name] == pkg_template:
                matched = True
                break

            if matched:
              filtered_lines.append(line)

          with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as t:
            t.writelines(filtered_lines)
            ensure_file_to_use = t.name
          _info('Using filtered ensure file: %s' % ensure_file_to_use)

        cipd_path = os.environ.get('VPYTHON_CIPD_PATH', 'cipd')
        cipd_cmd = [cipd_path]
        if sys.platform == 'win32' and cipd_cmd[0].lower().endswith('.bat'):
          cipd_cmd = ['cmd.exe', '/C'] + cipd_cmd

        try:
          subprocess.check_call(cipd_cmd + [
              'export', '-ensure-file',
              ensure_file_to_use, '-root', wheels_cache
          ])
        except subprocess.CalledProcessError:
          msg = 'FATAL: One or more requirements are missing from BOTH AR and CIPD: %s' % failed_requirements
          _info(msg)
          sys.stderr.write(msg + '\n')
          raise

        if ensure_file_to_use != ensure_file:
          try:
            os.unlink(ensure_file_to_use)
          except OSError:
            pass

    _info('Installing mixed requirements as a single batch...')
    find_links_args = ['--find-links', wheels_dir]
    if os.path.isdir(os.path.join(os.environ['wheels'], 'wheels')):
      find_links_args += ['--find-links', os.path.join(os.environ['wheels'], 'wheels')]

    subprocess.check_call([
        pip,
        'install',
        '--isolated',
        '--compile',
        '--no-index',
        '--no-deps',
    ] + find_links_args + ['--requirement', requirements_file])

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
