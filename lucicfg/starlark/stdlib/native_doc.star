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

"""Some extra documentation.

This module is not imported by anything. It exists only to document public
native symbols exposed by `lucicfg` go code in the global namespace of all
modules.
"""

# TODO(vadimsh): Figure out how to document 'load'. The AST parser refuses to
# parse a function named 'load' :(
#
# @unused
def load__(module, *args, **kwargs):
    """Loads a Starlark module as a library (if it hasn't been loaded before).

    Extracts one or more values from it, and binds them to names in the current
    module.

    A load statement requires at least two "arguments". The first must be a
    literal string, it identifies the module to load. The remaining arguments
    are a mixture of literal strings, such as `'x'`, or named literal strings,
    such as `y='x'`.

    The literal string (`'x'`), which must denote a valid identifier not
    starting with `_`, specifies the name to extract from the loaded module. In
    effect, names starting with `_` are not exported. The name (`y`) specifies
    the local name. If no name is given, the local name matches the quoted name.

    ```
    load('//module.star', 'x', 'y', 'z')       # assigns x, y, and z
    load('//module.star', 'x', y2='y', 'z')    # assigns x, y2, and z
    ```

    A load statement within a function is a static error.

    See also [Modules and packages](#modules-and-packages) for how load(...)
    interacts with exec(...).

    Args:
      module: module to load, i.e. `//path/within/current/package.star` or
        `@<pkg>//path/within/pkg.star` or `./relative/path.star`. Required.
      *args: what values to import under their original names.
      **kwargs: what values to import and bind under new names.
    """
    _unused(module, args, kwargs)

def exec(module):
    """Executes another Starlark module for its side effects.

    See also [Modules and packages](#modules_and_packages) for how load(...)
    interacts with exec(...).

    Args:
      module: module to execute, i.e. `//path/within/current/package.star` or
        `@<pkg>//path/within/pkg.star` or `./relative/path.star`. Required.

    Returns:
      A struct with all exported symbols of the executed module.
    """
    _unused(module)

def fail(msg, trace = None):
    """Aborts the execution with an error message.

    Args:
      msg: the error message string. Required.
      trace: a custom trace, as returned by stacktrace(...) to attach to
        the error. This may be useful if the root cause of the error is far
        from where `fail` is called.
    """
    _unused(msg, trace)

def stacktrace(skip = None):
    """Captures and returns a stack trace of the caller.

    A captured stacktrace is an opaque object that can be stringified to get a
    nice looking trace (e.g. for error messages).

    Args:
      skip: how many innermost call frames to skip. Default is 0.
    """
    _unused(skip)

def struct(**kwargs):
    """Returns an immutable struct object with given fields.

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
    _unused(kwargs)

def to_json(value):
    """Serializes a value to a compact JSON string.

    Doesn't support integers that do not fit int64. Fails if the value has
    cycles.

    *** note
    **Deprecated.** Use json.encode(...) instead. Note that json.encode(...)
    will retain the order of dict keys, unlike to_json(...) that always sorts
    them alphabetically.
    ***

    Args:
      value: a primitive Starlark value: a scalar, or a list/tuple/dict
        containing only primitive Starlark values. Required.
    """
    _unused(value)

def _unused(*args):  # @unused
    """Used exclusively to shut up `unused-variable` lint.

    DocTags:
      Hidden.
    """
