// Copyright 2021 The LUCI Authors.
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

import { isArrayBuffer } from 'lodash-es';

/**
 * An object that represents a body. It can by easily
 * 1. serialized to json string.
 * 2. deserialized to the original request/response body.
 */
export type BodyRepresentation = Base64Body | StringBody | JsonBody;

export interface Base64Body {
  type: 'base64';
  data: string;
}

export interface StringBody {
  type: 'string';
  data: string;
}
export interface JsonBody {
  type: 'json';
  data: object;
}

/**
 * Converts a request/response body to a BodyRepresentation.
 */
export function toBodyRep(body: ArrayBuffer | string | object): BodyRepresentation {
  if (isArrayBuffer(body)) {
    return {
      type: 'base64',
      data: Buffer.from(new Uint8Array(body)).toString('base64'),
    };
  } else if (typeof body === 'string') {
    return {
      type: 'string',
      data: body,
    };
  } else {
    return {
      type: 'json',
      data: body,
    };
  }
}

/**
 * Converts a BodyRepresentation to the original value.
 */
export function fromBodyRep(bodyRep: BodyRepresentation): ArrayBuffer | string | object {
  switch (bodyRep.type) {
    case 'base64':
      return new Uint8Array(Buffer.from(bodyRep.data, 'base64')).buffer;
    case 'string':
      return bodyRep.data as string;
    case 'json':
      return bodyRep.data;
  }
}
