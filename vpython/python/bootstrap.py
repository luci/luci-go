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
  COMPILE_WORKERS = os.cpu_count() or 1
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
  pip = glob.glob(os.path.join(os.environ['out'], '*', 'pip*'))[0]
  subprocess.check_call([
      pip,
      'install',
      '--isolated',
      '--compile',
      '--no-index',
      '--find-links',
      os.path.join(os.environ['wheels'], 'wheels'),
      '--requirement',
      os.path.join(os.environ['wheels'], 'requirements.txt'),
  ])

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
