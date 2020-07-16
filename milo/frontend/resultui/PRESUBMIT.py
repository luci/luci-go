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

"""ResultUI presubmit script.

See https://dev.chromium.org/developers/how-tos/depottools/presubmit-scripts for
details on the presubmit API built into depot_tools.
"""

import os
import re
import sys


def RunTests(input_api, output_api):
  """Runs tests in the directory."""
  if input_api.is_committing:
    error_type = output_api.PresubmitError
  else:
    error_type = output_api.PresubmitPromptWarning

  output = []
  output += input_api.RunTests([input_api.Command(
    name='resultui presubmit',
    cmd=[
      'npm',
      'run',
      'test',
    ],
    kwargs={},
    message=error_type,
  )])
  return output

def CommonChecks(input_api, output_api):
  results = []
  results.extend(RunTests(input_api, output_api))
  return results


def CheckChangeOnUpload(input_api, output_api):
  return CommonChecks(input_api, output_api)


def CheckChangeOnCommit(input_api, output_api):
  return CommonChecks(input_api, output_api)
