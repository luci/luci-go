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
  """Returns a duration that represents the integer number of milliseconds."""
  if type(milliseconds) != 'int':
    fail('time.duration: got %s, want int' % type(milliseconds))
  return __native__.make_duration(milliseconds)


def _truncate(duration, precision):
  """Truncates the precision of the duration to the given value.

  For example time.truncate(time.hour+10*time.minute, time.hour) is time.hour.
  """
  if type(duration) != 'duration':
    fail('time.truncate: got %s as first argument, want duration' % type(duration))
  if type(precision) != 'duration':
    fail('time.truncate: got %s as second argument, want duration' % type(precision))
  return (duration / precision) * precision


# Time module provides a simple API for defining durations in a readable way,
# resembling golang's time.Duration.
#
# Durations are represented by integer-like values of 'duration' type, which
# internally hold a number of milliseconds.
#
# Durations can be added and subtracted from each other and multiplied by
# integers to get durations. They are also comparable to each other (but not
# to integers). Durations can also be divided by each other to get an integer,
# e.g. time.hour / time.second produces 3600.
#
# The best way to define a duration is to multiply an integer by a corresponding
# "unit" constant, for example 10 * time.second.
time = struct(
    duration = _duration,
    truncate = _truncate,

    zero = _duration(0),
    millisecond = _duration(1),
    second = _duration(1000),
    minute = _duration(60 * 1000),
    hour = _duration(60 * 60 * 1000),
    day = _duration(24 * 60 * 60 * 1000),
)
