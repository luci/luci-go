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

"""Generic value validators."""

load('@stdlib//internal/re.star', 're')
load('@stdlib//internal/time.star', 'time')


def _string(attr, val, regexp=None, default=None, required=True):
  """Validates that the value is a string and returns it.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    regexp: a regular expression to check 'val' against.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The validated string or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  if type(val) != 'string':
    fail('bad %r: got %s, want string' % (attr, type(val)))

  if regexp and not re.submatches(regexp, val):
    fail('bad %r: %r should match %r' % (attr, val, regexp))

  return val


def _int(attr, val, min=None, max=None, default=None, required=True):
  """Validates that the value is an integer and returns it.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    min: minimal allowed value (inclusive) or None for unbounded.
    max: maximal allowed value (inclusive) or None for unbounded.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The validated int or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  if type(val) != 'int':
    fail('bad %r: got %s, want int' % (attr, type(val)))

  if min != None and val < min:
    fail('bad %r: %s should be >= %s' % (attr, val, min))
  if max != None and val > max:
    fail('bad %r: %s should be <= %s' % (attr, val, max))

  return val


def _float(attr, val, min=None, max=None, default=None, required=True):
  """Validates that the value is a float or integer and returns it as float.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    min: minimal allowed value (inclusive) or None for unbounded.
    max: maximal allowed value (inclusive) or None for unbounded.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The validated float or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  if type(val) == 'int':
    val = float(val)
  elif type(val) != 'float':
    fail('bad %r: got %s, want float or int' % (attr, type(val)))

  if min != None and val < min:
    fail('bad %r: %s should be >= %s' % (attr, val, min))
  if max != None and val > max:
    fail('bad %r: %s should be <= %s' % (attr, val, max))

  return val


def _bool(attr, val, default=None, required=True):
  """Validates that the value can be converted to a boolean.

  Zero values other than None (0, "", [], etc) are treated as False. None
  indicates "use default". If required is False and val is None, returns None
  (indicating no value was passed).

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The boolean or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default
  return bool(val)


def _duration(attr, val, precision=time.second, min=time.zero, max=None, default=None, required=True):
  """Validates that the value is a duration specified at the given precision.

  For example, if 'precision' is time.second, will validate that the given
  duration has a whole number of seconds. Fails if truncating the duration to
  the requested precision loses information.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    precision: a time unit to divide 'val' by to get the output.
    min: minimal allowed duration (inclusive) or None for unbounded.
    max: maximal allowed duration (inclusive) or None for unbounded.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The validated duration or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  if type(val) != 'duration':
    fail('bad %r: got %s, want duration' % (attr, type(val)))

  if min != None and val < min:
    fail('bad %r: %s should be >= %s' % (attr, val, min))
  if max != None and val > max:
    fail('bad %r: %s should be <= %s' % (attr, val, max))

  if time.truncate(val, precision) != val:
    fail((
        'bad %r: losing precision when truncating %s to %s units, ' +
        'use time.truncate(...) to acknowledge') % (attr, val, precision))

  return val


def _list(attr, val, required=False):
  """Validates that the value is a list and returns it.

  None is treated as an empty list.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    required: if False, allow 'val' to be None or empty, return empty list in
        this case.

  Returns:
    The validated list.
  """
  if val == None:
    val = []

  if type(val) != 'list':
    fail('bad %r: got %s, want list' % (attr, type(val)))

  if required and not val:
    fail('missing required field %r' % attr)

  return val


def _str_dict(attr, val, required=False):
  """Validates that the value is a dict with non-empty string keys.

  None is treated as an empty dict.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    required: if False, allow 'val' to be None or empty, return empty dict in
        this case.

  Returns:
    The validated dict.
  """
  if val == None:
    val = {}

  if type(val) != 'dict':
    fail('bad %r: got %s, want dict' % (attr, type(val)))

  if required and not val:
    fail('missing required field %r' % attr)

  for k in val:
    if type(k) != 'string':
      fail('bad %r: got %s key, want string' % (attr, type(k)))
    if not k:
      fail('bad %r: got empty key' % attr)

  return val


def _struct(attr, val, sym, default=None, required=True):
  """Validates that the value is a struct of the given flavor and returns it.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    sym: a name of the constructor that produced the struct.
    default: a value to use if 'val' is None, ignored if required is True.
    required: if False, allow 'val' to be None, return 'default' in this case.

  Returns:
    The validated struct or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  tp = ctor(val) or type(val)  # ctor(...) return None for non-structs
  if tp != sym:
    fail('bad %r: got %s, want %s' % (attr, tp, sym))

  return val


def _type(attr, val, prototype, default=None, required=True):
  """Validates that the value is either None or has the same type as `prototype`
  value.

  Useful when checking types of protobuf messages.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    prototype: a prototype value to compare val's type against.
    default: a value to use if `val` is None, ignored if required is True.
    required: if False, allow `val` to be None, return `default` in this case.

  Returns:
    `val` on success or None if required is False and default is None.
  """
  if val == None:
    if required:
      fail('missing required field %r' % attr)
    if default == None:
      return None
    val = default

  if type(val) != type(prototype):
    fail('bad %r: got %s, want %s' % (attr, type(val), type(prototype)))

  return val


validate = struct(
    string = _string,
    int = _int,
    float = _float,
    bool = _bool,
    duration = _duration,
    list = _list,
    str_dict = _str_dict,
    struct = _struct,
    type = _type,
)
