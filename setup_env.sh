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

# This script is a wrapper for running eval `../../../env.py` due to
# safeguards that exist within Gemini CLI that prevent use of command
# substitution in shell commands:
# https://github.com/google-gemini/gemini-cli/blob/e90e0015eacda0cf8315e30b72305e026f3768b3/packages/core/src/tools/shell.ts#L119
#
# Usage: `. ./setup_env.py` (note the dot space script syntax
# so that environment variable 'stick').

#!/bin/bash

eval `../../../env.py`