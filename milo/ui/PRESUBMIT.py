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
        f, files_to_check=[r'.+\.(ts|tsx|js|jsx)$'])

  affected_files = input_api.AffectedFiles(file_filter=file_filter)
  if not affected_files:
    return []

  # We run these commands in the directory of this PRESUBMIT.py file.
  cwd = input_api.PresubmitLocalPath()

  # Optimizing Lint: Run eslint ONLY on affected files.
  # We use 'npx eslint' to ensure we use the local eslint installation.
  # We pass the relative paths of affected files.
  affected_file_paths = [
      input_api.os_path.relpath(f.AbsoluteLocalPath(), cwd)
      for f in affected_files
  ]

  # For type-check, we must still run the full check, but we rely on
  # 'incremental': true in tsconfig.json to speed it up.

  tests = []

  # Lint command
  tests.append(input_api.Command(
      name='eslint (affected files)',
      cmd=['npx', 'eslint'] + affected_file_paths,
      kwargs={'cwd': cwd},
      message=output_api.PresubmitError))

  # Type check command
  tests.append(input_api.Command(
      name='npm run type-check',
      cmd=['npm', 'run', 'type-check'],
      kwargs={'cwd': cwd},
      message=output_api.PresubmitError))

  return input_api.RunTests(tests)


def CheckChangeOnUpload(input_api, output_api):
  return CheckLintAndTypes(input_api, output_api)


def CheckChangeOnCommit(input_api, output_api):
  return CheckLintAndTypes(input_api, output_api)
