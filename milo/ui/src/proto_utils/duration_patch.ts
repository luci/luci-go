// Copyright 2024 The LUCI Authors.
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

/**
 * @fileoverview
 * ts-proto does not use the canonical JSON encoding for the duration object.
 * This module patches the proto message class to use the canonical JSON
 * encoding so we can use JSON encoding when calling pRPC endpoints.
 * This will no longer be needed once ts-proto supports the canonical Duration
 * JSON encoding.
 */

import { Duration } from '@/proto/google/protobuf/duration.pb';

// Use the same signature as the code generated function.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
Duration.fromJSON = (object: any): Duration => {
  if (typeof object === 'string') {
    const [seconds, decimal = ''] = object
      .substring(0, object.length - 1)
      .split('.');
    const nanos = globalThis.Number('0.' + decimal) * 1_000_000_000;

    const sign = seconds.startsWith('-') ? -1 : 1;
    return {
      // Use '0' instead of '-0' when the duration is less than 1 second.
      // See the documentation for `nanos`.
      seconds: seconds === '-0' ? '0' : seconds,
      nanos: sign * nanos,
    };
  }

  return {
    seconds: isSet(object.seconds) ? globalThis.String(object.seconds) : '0',
    nanos: isSet(object.nanos) ? globalThis.Number(object.nanos) : 0,
  };
};

Duration.toJSON = (message: Duration): unknown => {
  const decimal = (Math.abs(message.nanos) / 1_000_000_000)
    .toFixed(9)
    .replace(/\.?0+$/, '')
    .substring(2);
  const additionalSign =
    message.seconds === '0' && message.nanos < 0 ? '-' : '';
  return `${additionalSign}${message.seconds}${decimal ? '.' : ''}${decimal}s`;
};

// Use the same signature as the code generated function.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
