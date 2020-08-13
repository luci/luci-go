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

import { DateTime } from 'luxon';
import { Timestamp } from '../services/buildbucket';

export function displayTimestamp(t: Timestamp): string {
    const d = datetimeFromTimestamp(t);
    return d.toFormat("ccc, HH:mm:ss MMM dd yyyy ZZZZ");
}

export function displayDuration(beginTime: Timestamp, endTime: Timestamp): string {
    const bd = datetimeFromTimestamp(beginTime);
    const ed = datetimeFromTimestamp(endTime);
    const duration = ed.diff(bd).shiftTo("days", "hours", "minutes", "seconds", "milliseconds");
    let parts = [];
    if (duration.days >= 1) {
        parts.push(duration.days + " " + (duration.days == 1 ? "day":"days"));
    }
    if (duration.hours >= 1) {
        parts.push(duration.hours + " " + (duration.hours == 1 ? "hour":"hours"));
    }
    // We only care about minutes if days and hours are not both present
    if (duration.minutes >= 1 && parts.length <= 1) {
        parts.push(duration.minutes + " " + (duration.minutes == 1 ? "min":"mins"));
    }
    // We only care about seconds if it is significant enough
    if (duration.seconds >= 1 && parts.length <= 1) {
        parts.push(duration.seconds + " " + (duration.seconds == 1 ? "sec":"secs"));
    }
    // We only care about ms if there are no other part
    if (duration.milliseconds > 0 && parts.length == 0) {
        parts.push(Math.floor(duration.milliseconds) + " ms");
    }
    return parts.join(" ");
}

function datetimeFromTimestamp(t: Timestamp): DateTime {
    return DateTime.fromMillis(t.seconds * 1000 + t.nanos / 1000000);
}