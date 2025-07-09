// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { Duration } from 'luxon';

import {
  displayCompactDuration,
  displayDuration,
  displayApproxDuration,
} from './time_utils';

describe('displayDuration', () => {
  it('should display correct duration in days and hours', async () => {
    const duration = Duration.fromISO('P3DT11H');
    expect(displayDuration(duration)).toStrictEqual('3 days 11 hours');
  });
  it('should display correct duration in hours and minutes', async () => {
    const duration = Duration.fromISO('PT1H11M');
    expect(displayDuration(duration)).toStrictEqual('1 hour 11 mins');
  });
  it('should display correct duration in minutes and seconds', async () => {
    const duration = Duration.fromISO('PT2M3S');
    expect(displayDuration(duration)).toStrictEqual('2 mins 3 secs');
  });
  it('should display correct duration in seconds', async () => {
    const duration = Duration.fromISO('PT15S');
    expect(displayDuration(duration)).toStrictEqual('15 secs');
  });
  it('should display correct duration in milliseconds', async () => {
    const duration = Duration.fromMillis(999);
    expect(displayDuration(duration)).toStrictEqual('999 ms');
  });
  it('should ignore milliseconds if duration > 1 second', async () => {
    const duration = Duration.fromMillis(1999);
    expect(displayDuration(duration)).toStrictEqual('1 sec');
  });
  it('should display zero duration correctly', async () => {
    const duration = Duration.fromMillis(0);
    expect(displayDuration(duration)).toStrictEqual('0 ms');
  });
  it('should display negative duration correctly', async () => {
    const duration = Duration.fromMillis(-1999);
    expect(displayDuration(duration)).toStrictEqual('0 ms');
  });
});

describe('displayCompactDuration', () => {
  it("should display correct duration when it's null", async () => {
    expect(displayCompactDuration(null)).toEqual(['N/A', '']);
  });
  it('should display correct duration in days', async () => {
    const duration = Duration.fromISO('P3DT11H');
    expect(displayCompactDuration(duration)).toEqual(['3.5d', 'd']);
  });
  it('should display correct duration in hours', async () => {
    const duration = Duration.fromISO('PT1H11M');
    expect(displayCompactDuration(duration)).toEqual(['1.2h', 'h']);
  });
  it('should display correct duration in minutes', async () => {
    const duration = Duration.fromISO('PT2M3S');
    expect(displayCompactDuration(duration)).toEqual(['2.0m', 'm']);
  });
  it('should display correct duration in seconds', async () => {
    const duration = Duration.fromISO('PT15S');
    expect(displayCompactDuration(duration)).toEqual(['15s', 's']);
  });
  it('should display correct duration in milliseconds', async () => {
    const duration = Duration.fromMillis(999);
    expect(displayCompactDuration(duration)).toEqual(['999ms', 'ms']);
  });
  it("should not display the duration in ms if it's no less than 999.5ms", async () => {
    const duration = Duration.fromMillis(999.5);
    expect(displayCompactDuration(duration)).toEqual(['1.0s', 's']);
  });
  it("should display the duration in ms if it's less than 999.5ms", async () => {
    const duration = Duration.fromMillis(999.4999);
    expect(displayCompactDuration(duration)).toEqual(['999ms', 'ms']);
  });
  it('should display negative duration as 0', async () => {
    const duration = Duration.fromISO('-PT15S');
    expect(displayCompactDuration(duration)).toEqual(['0ms', 'ms']);
  });
});

describe('displayApproxDuration', () => {
  it('should return "N/A" for null duration', () => {
    expect(displayApproxDuration(null)).toBe('N/A');
  });

  it.each([
    [Duration.fromObject({ seconds: 0 }), 'a few seconds'],
    [Duration.fromObject({ seconds: 30 }), 'a few seconds'],
    [Duration.fromObject({ seconds: 44 }), 'a few seconds'],
  ])(
    'should handle seconds less that 45 returning "a few seconds"',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ seconds: 45 }), 'a minute'],
    [Duration.fromObject({ seconds: 60 }), 'a minute'],
    [Duration.fromObject({ seconds: 89 }), 'a minute'],
  ])(
    'should return "a minute" for durations between 45 and 89 seconds',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ minutes: 1.8 }), '2 minutes'],
    [Duration.fromObject({ minutes: 20 }), '20 minutes'],
    [Duration.fromObject({ minutes: 44 }), '44 minutes'],
  ])(
    'should return "X minutes" for durations between 90 seconds and 44 minutes',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ minutes: 45 }), 'an hour'],
    [Duration.fromObject({ minutes: 75 }), 'an hour'],
    [Duration.fromObject({ minutes: 89 }), 'an hour'],
  ])(
    'should return "an hour" for durations between 45 and 89 minutes',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ hours: 1.8 }), '2 hours'],
    [Duration.fromObject({ hours: 10 }), '10 hours'],
    [Duration.fromObject({ hours: 21 }), '21 hours'],
  ])(
    'should return "X hours" for durations between 90 minutes and 21 hours',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ hours: 22 }), 'a day'],
    [Duration.fromObject({ hours: 30 }), 'a day'],
    [Duration.fromObject({ hours: 35 }), 'a day'],
  ])(
    'should return "a day" for durations between 22 and 35 hours',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ days: 1.8 }), '2 days'],
    [Duration.fromObject({ days: 15 }), '15 days'],
    [Duration.fromObject({ days: 25 }), '25 days'],
  ])(
    'should return "X days" for durations between 36 hours and 25 days',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ days: 26 }), 'a month'],
    [Duration.fromObject({ days: 30 }), 'a month'],
    [Duration.fromObject({ days: 45 }), 'a month'],
  ])(
    'should return "a month" for durations between 26 and 45 days',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ days: 46 }), '2 months'],
    [Duration.fromObject({ days: 100 }), '3 months'],
    [Duration.fromObject({ days: 304 }), '10 months'],
  ])(
    'should return "X months" for durations between 46 and 304 days',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ days: 305 }), 'a year'],
    [Duration.fromObject({ days: 400 }), 'a year'],
    [Duration.fromObject({ days: 547 }), 'a year'],
  ])(
    'should return "a year" for durations between 305 and 547 days',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );

  it.each([
    [Duration.fromObject({ days: 548 }), '2 years'],
    [Duration.fromObject({ days: 1000 }), '3 years'],
    [Duration.fromObject({ days: 2200 }), '6 years'],
  ])(
    'should return "X years" for durations greater than 547 days',
    (duration, expected) => {
      expect(displayApproxDuration(duration)).toBe(expected);
    },
  );
});
