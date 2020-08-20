# Copyright 2015 The LUCI Authors.
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
  allowed_years = (str(s) for s in reversed(xrange(2011, current_year + 1)))
  years_re = '(' + '|'.join(allowed_years) + ')'
  lines = [
    ('.*? ' + re.escape(line)) if line else '.*?'
    for line in COPYRIGHT_TEMPLATE.splitlines()
  ]
  lines[0] = lines[0].replace('YEARPATTERN', years_re)
  return '\n'.join(lines) + '(?: \*/)?\n'


def source_file_filter(input_api):
  """Returns filter that selects source code files only."""
  files_to_skip = list(input_api.DEFAULT_FILES_TO_SKIP) + [
    r'.+\.pb\.go$',
    r'.+_string\.go$',
  ]
  files_to_check = list(input_api.DEFAULT_FILES_TO_CHECK) + [
    r'.+\.go$',
  ]
  return lambda x: input_api.FilterSourceFile(
      x, files_to_check=files_to_check, files_to_skip=files_to_skip)


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
