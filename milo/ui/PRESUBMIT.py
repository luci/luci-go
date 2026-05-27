# Copyright 2025 The LUCI Authors.
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

"""Presubmit script for LUCI UI."""

PRESUBMIT_VERSION = '2.0.0'


def CheckLintAndTypes(input_api, output_api):
  """Runs npm run lint and npm run type-check if relevant files changed."""
  # Only check if relevant files are changed to avoid slowing down other CLs.
  def file_filter(f):
    return input_api.FilterSourceFile(
        f, files_to_check=[r'.+\.(ts|tsx|js|jsx)$', r'.*package(-lock)?\.json$'])

  affected_files = input_api.AffectedFiles(file_filter=file_filter)
  if not affected_files:
    return []

  # We run these commands in the directory of this PRESUBMIT.py file.
  cwd = input_api.PresubmitLocalPath()

  # Find the go/env.py wrapper robustly by walking up the directory tree from cwd.
  env_py = None
  curr = cwd
  for _ in range(25):
    # Try standard infra/go/env.py layout
    candidate = input_api.os_path.join(curr, 'infra', 'go', 'env.py')
    if input_api.os_path.exists(candidate):
      env_py = candidate
      break
    # Try standalone/nested repo layout where env.py is at a parent root
    candidate_direct = input_api.os_path.join(curr, 'env.py')
    if input_api.os_path.exists(candidate_direct):
      env_py = candidate_direct
      break
    # Go up one directory level
    parent = input_api.os_path.dirname(curr)
    if parent == curr: # reached filesystem root
      break
    curr = parent

  npm_cmd = ['npm']
  npx_cmd = ['npx']
  if env_py:
    npm_cmd = [input_api.python3_executable, env_py, 'npm']
    npx_cmd = [input_api.python3_executable, env_py, 'npx']

  package_json_path = input_api.os_path.join(cwd, 'package.json')
  # Check if node_modules exists and is up to date with package-lock.json and package.json.
  node_modules_path = input_api.os_path.join(cwd, 'node_modules')
  package_lock_path = input_api.os_path.join(cwd, 'package-lock.json')
  stamp_path = input_api.os_path.join(node_modules_path, '.last-install-stamp')

  needs_install = False
  if not input_api.os_path.exists(node_modules_path):
    print('node_modules not found in %s, installing dependencies...' % cwd)
    needs_install = True
  elif input_api.os_path.exists(package_lock_path):
    if not input_api.os_path.exists(stamp_path):
      print('node_modules last install stamp not found in %s, installing...' % cwd)
      needs_install = True
    else:
      stamp_mtime = input_api.os_path.getmtime(stamp_path)
      lock_mtime = input_api.os_path.getmtime(package_lock_path)
      json_mtime = (input_api.os_path.getmtime(package_json_path)
                    if input_api.os_path.exists(package_json_path) else 0)
      if lock_mtime > stamp_mtime or json_mtime > stamp_mtime:
        print('node_modules is stale in %s, updating dependencies...' % cwd)
        needs_install = True

  if needs_install:
    try:
      # Check if npm is available
      input_api.subprocess.check_call(
          npm_cmd + ['--version'], stdout=input_api.subprocess.DEVNULL, stderr=input_api.subprocess.DEVNULL)
      # Run npm ci (allow stderr to print to console on failure)
      input_api.subprocess.check_call(
          npm_cmd + ['ci'], cwd=cwd, stdout=input_api.subprocess.DEVNULL)
      # Touch the stamp file to mark successful installation
      if input_api.os_path.exists(node_modules_path):
        try:
          with open(stamp_path, 'w') as f:
            f.write('')
        except OSError as e:
          print('Warning: Failed to write dependency installation stamp file to %s: %s\n'
                'Presubmit will run "npm ci" again on subsequent executions if dependencies remain out of sync.' % (stamp_path, e))
    except (OSError, input_api.subprocess.CalledProcessError) as e:
      return [output_api.PresubmitError(
          'Failed to install/update dependencies in %s.\n'
          'Detailed error: %s\n'
          'If you recently modified package.json, please make sure to run '
          '"npm install" locally to update package-lock.json and commit the changes.' % (cwd, e))]

  # Optimizing Lint: Run eslint ONLY on affected JS/TS files.
  # We use 'npx eslint' to ensure we use the local eslint installation.
  # We pass the relative paths of affected files.
  affected_js_ts_files = [
      input_api.os_path.relpath(f.AbsoluteLocalPath(), cwd)
      for f in affected_files
      if f.Action() != 'D' and f.LocalPath().endswith(('.ts', '.tsx', '.js', '.jsx'))
  ]

  # For type-check, we must still run the full check, but we rely on
  # 'incremental': true in tsconfig.json to speed it up.
  tests = []

  # Lint command
  if affected_js_ts_files:
    tests.append(input_api.Command(
        name='eslint (affected files)',
        cmd=npx_cmd + ['eslint'] + affected_js_ts_files,
        kwargs={'cwd': cwd},
        message=output_api.PresubmitError))

  # Type check command
  tests.append(input_api.Command(
      name='npm run type-check',
      cmd=npm_cmd + ['run', 'type-check'],
      kwargs={'cwd': cwd},
      message=output_api.PresubmitError))

  return input_api.RunTests(tests)


def CheckChangeOnUpload(input_api, output_api):
  return CheckLintAndTypes(input_api, output_api)


def CheckChangeOnCommit(input_api, output_api):
  return CheckLintAndTypes(input_api, output_api)
