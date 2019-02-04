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


# TODO(vadimsh): Figure out how to document 'load'. The AST parser refuses to
# parse a function named 'load' :(
def __load(module, *args, **kwargs):
  """Loads another Starlark module (if it haven't been loaded before), extracts
  one or more values from it, and binds them to names in the current module.

  This is not actually a function, but a core Starlark statement. We give it
  additional meaning (related to [the execution model](#execution_doc)) worthy
  of additional documentation.

  A load statement requires at least two "arguments". The first must be a
  literal string, it identifies the module to load. The remaining arguments are
  a mixture of literal strings, such as `'x'`, or named literal strings, such as
  `y='x'`.

  The literal string (`'x'`), which must denote a valid identifier not starting
  with `_`, specifies the name to extract from the loaded module. In effect,
  names starting with `_` are not exported. The name (`y`) specifies the local
  name. If no name is given, the local name matches the quoted name.

  ```
  load('//module.star', 'x', 'y', 'z')       # assigns x, y, and z
  load('//module.star', 'x', y2='y', 'z')    # assigns x, y2, and z
  ```

  A load statement within a function is a static error.

  TODO(vadimsh): Write about 'load' and 'exec' contexts and how 'load' and
  'exec' interact with each other.

  Args:
    module: module to load, i.e. `//path/within/current/package.star` or
        `@<pkg>//path/within/pkg.star`. Required.
  """


def exec(module):
  """Executes another Starlark module for its side effects.

  TODO(vadimsh): Write about 'load' and 'exec' contexts and how 'load' and
  'exec' interact with each other.

  Args:
    module: module to execute, i.e. `//path/within/current/package.star` or
        `@<pkg>//path/within/pkg.star`. Required.

  Returns:
    A struct with all exported symbols of the executed module.
  """


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
