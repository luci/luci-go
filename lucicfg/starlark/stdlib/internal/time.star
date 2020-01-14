# Copyright 2019 The LUCI Authors.
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


def _duration(milliseconds):
  """Returns a duration that represents the integer number of milliseconds.

  Args:
    milliseconds: integer with the requested number of milliseconds. Required.

  Returns:
    time.duration value.
  """
  if type(milliseconds) != 'int':
    fail('time.duration: got %s, want int' % type(milliseconds))
  return __native__.make_duration(milliseconds)


def _epoch(layout, value, location):
  """Returns epoch seconds for value interpreted as a time per layout in location.

  Args:
    layout: a string format showing how the reference time would be
      interpreted, see golang's time.Parse. Required.
    value: a string value to be parsed as a time. Required.
    location: a string location, for example 'America/Los_Angeles'. Required.

  Returns:
    int epoch seconds for value.
  """
  if type(layout) != 'string':
    fail('time.epoch: got %s as first argument, want string' % type(layout))
  if type(value) != 'string':
    fail('time.epoch: got %s as second argument, want string' % type(value))
  if type(location) != 'string':
    fail('time.epoch: got %s as third argument, want string' % type(location))
  return __native__.epoch(layout, value, location)


def _truncate(duration, precision):
  """Truncates the precision of the duration to the given value.

  For example `time.truncate(time.hour+10*time.minute, time.hour)` is
  `time.hour`.

  Args:
    duration: a time.duration to truncate. Required.
    precision: a time.duration with precision to truncate to. Required.

  Returns:
    Truncated time.duration value.
  """
  if type(duration) != 'duration':
    fail('time.truncate: got %s as first argument, want duration' % type(duration))
  if type(precision) != 'duration':
    fail('time.truncate: got %s as second argument, want duration' % type(precision))
  return (duration / precision) * precision


_days = {'mon': 1, 'tue': 2, 'wed': 3, 'thu': 4, 'fri': 5, 'sat': 6, 'sun': 7}


def _day_index(name):
  """E.g. 'Tue' -> 1."""
  idx = _days.get(name.lower())
  if idx == None:
    fail('days_of_week: %r is not a valid 3-char abbreviated day of the week' % (name,))
  return idx


def _days_of_week(spec):
  """Parses e.g. `Tue,Fri-Sun` into a list of day indexes, e.g. `[2, 5, 6, 7]`.

  Monday is 1, Sunday is 7. The returned list is sorted and has no duplicates.
  An empty string results in the empty list.

  Args:
    spec: a case-insensitive string with 3-char abbreviated days of the week.
        Multiple terms are separated by a comma and optional spaces. Each term
        is either a day (e.g. `Tue`), or a range (e.g. `Wed-Sun`). Required.

  Returns:
    A list of 1-based day indexes. Monday is 1.
  """
  days = []
  for term in spec.split(','):
    term = term.strip()
    if not term:
      continue
    if '-' in term:
      l, r = term.split('-')
      l, r = l.strip(), r.strip()
      li, ri = _day_index(l), _day_index(r)
      if li > ri:
        fail('days_of_week: bad range %r - %r is later than %r' % (term, l, r))
      days.extend(range(li, ri+1))
    else:
      days.append(_day_index(term))
  return sorted(set(days))


# Time module provides a simple API for defining durations in a readable way,
# resembling golang's time.Duration.
#
# Durations are represented by integer-like values of time.duration(...) type,
# which internally hold a number of milliseconds.
#
# Durations can be added and subtracted from each other and multiplied by
# integers to get durations. They are also comparable to each other (but not
# to integers). Durations can also be divided by each other to get an integer,
# e.g. `time.hour / time.second` produces 3600.
#
# The best way to define a duration is to multiply an integer by a corresponding
# "unit" constant, for example `10 * time.second`.
#
# Following time constants are exposed:
#
# | Constant           | Value (obviously)         |
# |--------------------|---------------------------|
# | `time.zero`        | `0 milliseconds`          |
# | `time.millisecond` | `1 millisecond`           |
# | `time.second`      | `1000 * time.millisecond` |
# | `time.minute`      | `60 * time.second`        |
# | `time.hour`        | `60 * time.minute`        |
# | `time.day`         | `24 * time.hour`          |
# | `time.week`        | `7 * time.day`            |
time = struct(
    duration = _duration,
    epoch = _epoch,
    truncate = _truncate,

    # Handy for epoch's layout argument.
    short_date = '2006-01-02 15:04:05',

    zero = _duration(0),
    millisecond = _duration(1),
    second = _duration(1000),
    minute = _duration(60 * 1000),
    hour = _duration(60 * 60 * 1000),
    day = _duration(24 * 60 * 60 * 1000),
    week = _duration(7 * 24 * 60 * 60 * 1000),

    days_of_week = _days_of_week,
)
