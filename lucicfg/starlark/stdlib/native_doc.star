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

# This module is not imported by anything. It exists only to document public
# native symbols exposed by `lucicfg` go code in the global namespace of all
# modules.


def fail(msg, trace=None):
  """Aborts the execution with an error message.

  Args:
    msg: the error message string. Required.
    trace: a custom trace, as returned by stacktrace(...) to attach to
        the error. This may be useful if the root cause of the error is far from
        where `fail` is called.
  """


def stacktrace(skip=None):
  """Captures and returns a stack trace of the caller.

  A captured stacktrace is an opaque object that can be stringified to get a
  nice looking trace (e.g. for error messages).

  Args:
    skip: how many innermost call frames to skip. Default is 0.
  """


def struct(**kwargs):
  """Returns an immutable struct object with fields populated from the specified
  keyword arguments.

  Can be used to define namespaces, for example:

  ```python
  def _func1():
    ...

  def _func2():
    ...

  exported = struct(
      func1 = _func1,
      func2 = _func2,
  )
  ```

  Then `_func1` can be called as `exported.func1()`.

  Args:
    **kwargs: fields to put into the returned struct object.
  """


def to_json(value):
  """Serializes a value to a compact JSON string.

  Doesn't support integers that do not fit int64. Fails if the value has cycles.

  Args:
    value: a primitive Starlark value: a scalar, or a list/tuple/dict containing
        only primitive Starlark values. Required.
  """
