# Copyright 2018 The LUCI Authors.
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

def error(msg, *args, **kwargs):
  """Emits an error message and continues the execution.

  If at the end of the execution there's at least one error recorded, the
  execution is considered failed.

  Either captures the current stack trace for the traceback or uses a
  previously captured one if it was passed via 'trace' keyword argument:

    trace = stacktrace()
    ...
    error('Boom, %s', 'arg', trace=trace)

  Args:
    msg: error message format string.
    *args: arguments for the format string.
    **kwargs: either empty of contains single 'trace' var with a stack trace.
  """
  trace = kwargs.pop('trace', None) or stacktrace(skip=1)
  if len(kwargs) != 0:
    fail('expecting trace=... kwargs only')
  __native__.emit_error(msg % args, trace)
