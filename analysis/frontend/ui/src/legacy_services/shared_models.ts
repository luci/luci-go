// Copyright 2022 The LUCI Authors.
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

import {
  GrpcError,
  RpcCode,
} from '@chopsui/prpc-client';

// Contains data models shared between multiple services.

export interface ClusterId {
  algorithm: string;
  id: string;
}

// MetricId represents the identifier of a LUCI Analysis metric.
export type MetricId = string;

export interface AssociatedBug {
  system: string;
  id: string;
  linkText: string;
  url: string;
}

export interface Changelist {
  // Gerrit hostname, e.g. "chromium-review.googlesource.com".
  host: string;

  // Change number, encoded as a string, e.g. "12345".
  change: string;

  // Patchset number, e.g. 1.
  patchset: number;
}

export interface VariantDef {
  [key: string]: string | undefined;
}

export interface Variant {
  def: VariantDef;
}

export interface VariantPair {
  key: string;
  value: string;
}

// variantAsPairs converts a variant (mapping from keys
// to values) into a set of key-value pairs. key-value
// pairs are returned in sorted key order.
export function variantAsPairs(v?: Variant): VariantPair[] {
  const result: VariantPair[] = [];
  if (v === undefined) {
    return result;
  }
  for (const key in v.def) {
    if (!Object.prototype.hasOwnProperty.call(v.def, key)) {
      continue;
    }
    const value = v.def[key] || '';
    result.push({ key: key, value: value });
  }
  result.sort((a, b) => {
    return a.key.localeCompare(b.key, 'en');
  });
  return result;
}

// unionVariant returns a partial variant which contains
// only the key-value pairs shared by both v1 and v2.
// While it intersects the key-value pairs in both v1 and v2,
// the result is a union in the sense that filtering on the
// partial variant yields a set of varaints that includes
// both v1 and v2.
export function unionVariant(v1?: Variant, v2?: Variant): Variant {
  const result: Variant = { def: {} };
  if (v1 === undefined || v2 === undefined) {
    return result;
  }
  for (const key in v1.def) {
    // Only keep keys that are both in v1 and v2.
    if (!Object.prototype.hasOwnProperty.call(v1.def, key)) {
      continue;
    }
    if (!Object.prototype.hasOwnProperty.call(v2.def, key)) {
      continue;
    }
    // Where the value at the key is the same in both v1 and v2.
    const value1 = v1.def[key] || '';
    const value2 = v2.def[key] || '';
    if (value1 == value2) {
      result.def[key] = value1;
    }
  }
  return result;
}

export function isRetriable(e: GrpcError): boolean {
  // The following codes indicate transient errors that are retriable. See:
  // https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/grpc/grpcutil/errors.go;l=176?q=codeToStatus&type=cs
  switch (e.code) {
    case RpcCode.INTERNAL:
      return true;
    case RpcCode.UNKNOWN:
      return true;
    case RpcCode.UNAVAILABLE:
      return true;
  }
  return false;
}

export function prpcRetrier(_failureCount: number, e: Error): boolean {
  if (e instanceof GrpcError && isRetriable(e)) {
    return true;
  }
  return false;
}
