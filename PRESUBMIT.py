# Copyright 2017 The LUCI Authors.
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

"""Top-level presubmit script.

See https://dev.chromium.org/developers/how-tos/depottools/presubmit-scripts for
details on the presubmit API built into depot_tools.
"""

import os
import re
import sys

USE_PYTHON3 = True

# Note that the URL http://www.apache.org/licenses/LICENSE-2.0
# is not indented.
COPYRIGHT_TEMPLATE = """
Copyright YEARPATTERN The LUCI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
""".strip()

def header(input_api):
  """Returns the expected license header regexp for this project."""
  current_year = int(input_api.time.strftime('%Y'))
  allowed_years = (str(s) for s in reversed(range(2011, current_year + 1)))
  years_re = '(' + '|'.join(allowed_years) + ')'
  # The regex below should accept the following comment styles at the beginning of the line:
  # 1) #  -- Python, Bash
  # 2) // -- Go, Javascript
  # 3) /* -- CSS
  # See comments on https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/4202089 for details.
  lines = [
    (r'[#/*]?[#/*]?[ \t]*' + re.escape(line)) if line else '.*?'
    for line in COPYRIGHT_TEMPLATE.splitlines()
  ]
  lines[0] = lines[0].replace('YEARPATTERN', years_re)
  return '\n'.join(lines) + '(?: \*/)?\n'


def source_file_filter(input_api):
  """Returns filter that selects source code files only."""
  files_to_skip = list(input_api.DEFAULT_FILES_TO_SKIP) + [
    r'.+/bootstrap/.*',  # third party
    r'.+/jquery/.*',  # third party
    r'.+/pb\.discovery\.go$',
    r'.+/pb\.discovery_test\.go$',
    r'.+\.pb\.go$',
    r'.+\.pb\.ts$',
    r'.+\.pb\.validate\.go$',
    r'.+\.pb_test\.go$',
    r'.+_dec\.go$',
    r'.+.mock\.go$',
    r'.+_mux\.go$',
    r'.+_string\.go$',
    r'.+gae\.py$',  # symlinks from outside
    r'common/api/internal/gensupport/.*', # third party
    r'common/goroutine/goroutine_id.go',
    r'common/terminal/.*', # third party
    r'server/static/bower_components/.*',  # third party
    r'server/static/upload/bower_components/.*',  # third party
  ]
  files_to_check = list(input_api.DEFAULT_FILES_TO_CHECK) + [
    r'.+\.go$',
  ]
  return lambda x: input_api.FilterSourceFile(
      x, files_to_check=files_to_check, files_to_skip=files_to_skip)


def CheckGoModTidy(input_api, output_api):
  root = input_api.change.RepositoryRoot()
  return input_api.RunTests([
    input_api.Command(
      name='go mod tidy',
      cmd=[
        input_api.python3_executable,
        os.path.join(root, 'scripts', 'check_go_mod_tidy.py'),
        root,
      ],
      kwargs={},
      message=output_api.PresubmitError)
  ])


def CheckGoLinterConfigs(input_api, output_api):
  root = input_api.change.RepositoryRoot()
  return input_api.RunTests([
    input_api.Command(
      name='regen_golangci_config.py --check',
      cmd=[
        input_api.python3_executable,
        os.path.join(root, 'scripts', 'regen_golangci_config.py'),
        '--check',
      ],
      kwargs={},
      message=output_api.PresubmitError)
  ])


def CheckGoogleapisInSync(input_api, output_api):
  root = input_api.change.RepositoryRoot()
  return input_api.RunTests([
    input_api.Command(
      name='Assert googleapis librariy is in sync',
      cmd=[
        input_api.python3_executable,
        os.path.join(root, 'scripts', 'check_googleapis_in_sync.py'),
        root,
      ],
      kwargs={},
      message=output_api.PresubmitError)
  ])


def CommonChecks(input_api, output_api):
  results = []
  results.extend(
    input_api.canned_checks.CheckChangeHasNoStrayWhitespace(
      input_api, output_api,
      source_file_filter=source_file_filter(input_api)))
  results.extend(
    input_api.canned_checks.CheckLicense(
      input_api, output_api, header(input_api),
      source_file_filter=source_file_filter(input_api)))
  if os.environ.get('GO111MODULE') != 'off':
    results.extend(CheckGoModTidy(input_api, output_api))
  results.extend(CheckGoLinterConfigs(input_api, output_api))
  results.extend(CheckGoogleapisInSync(input_api, output_api))
  return results


def CheckChangeOnUpload(input_api, output_api):
  return CommonChecks(input_api, output_api)


def CheckChangeOnCommit(input_api, output_api):
  results = CommonChecks(input_api, output_api)
  results.extend(input_api.canned_checks.CheckChangeHasDescription(
      input_api, output_api))
  results.extend(input_api.canned_checks.CheckDoNotSubmitInDescription(
      input_api, output_api))
  results.extend(input_api.canned_checks.CheckDoNotSubmitInFiles(
      input_api, output_api))
  return results
