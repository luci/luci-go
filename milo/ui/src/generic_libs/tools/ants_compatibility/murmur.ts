// Copyright 2025 The LUCI Authors.
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

const C1 = 0x87c37b91114253d5n;
const C2 = 0x4cf5ad432745937fn;
const MASK_64 = 0xffffffffffffffffn;

// bits.RotateLeft64 implementation for BigInt.
function rotateLeft64(val: bigint, shift: bigint): bigint {
  return ((val << shift) & MASK_64) | (val >> (64n - shift));
}

// finalizationMix implementation.
function finalizationMix(k: bigint): bigint {
  k = k ^ (k >> 33n);
  k = (k * 0xff51afd7ed558ccdn) & MASK_64;
  k = k ^ (k >> 33n);
  k = (k * 0xc4ceb9fe1a85ec53n) & MASK_64;
  k = k ^ (k >> 33n);
  return k;
}

/**
 * murmurHash128 sets the lower and upper half of the 128-bit result.
 * It implements the MurmurHash3 (64bit word size, 128-bit hash size) hashing algorithm.
 */
export function murmurHash128(data: Uint8Array, seed = 0): [bigint, bigint] {
  // The number of whole (16-byte) blocks in the data.
  const wholeBlockCount = Math.floor(data.length / 16);

  // h1 and h2 represent the lower and upper half of the 128-bit result.
  let h1 = BigInt(seed);
  let h2 = BigInt(seed);

  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);

  // Process all whole blocks.
  for (let i = 0; i < wholeBlockCount; i++) {
    let k1 = view.getBigUint64(i * 16, true);
    let k2 = view.getBigUint64(i * 16 + 8, true);

    // Mix the input and XOR it into the preliminary result.
    k1 = (k1 * C1) & MASK_64;
    k1 = rotateLeft64(k1, 31n);
    k1 = (k1 * C2) & MASK_64;
    h1 = h1 ^ k1;

    h1 = rotateLeft64(h1, 27n);
    h1 = (h1 + h2) & MASK_64;
    h1 = (h1 * 5n + 0x52dce729n) & MASK_64;

    k2 = (k2 * C2) & MASK_64;
    k2 = rotateLeft64(k2, 33n);
    k2 = (k2 * C1) & MASK_64;
    h2 = h2 ^ k2;

    h2 = rotateLeft64(h2, 31n);
    h2 = (h2 + h1) & MASK_64;
    h2 = (h2 * 5n + 0x38495ab5n) & MASK_64;
  }

  if (data.length % 16 > 0) {
    // Tail block. As this is a partial block, append zeroes to
    // pad it out to 16 bytes.
    const tailBlock = new Uint8Array(16);
    tailBlock.set(data.subarray(wholeBlockCount * 16));
    const tailView = new DataView(tailBlock.buffer);

    let k1 = tailView.getBigUint64(0, true);
    let k2 = tailView.getBigUint64(8, true);

    // Mix the input and XOR it into the preliminary result
    // (same as for whole blocks).
    k1 = (k1 * C1) & MASK_64;
    k1 = rotateLeft64(k1, 31n);
    k1 = (k1 * C2) & MASK_64;
    h1 = h1 ^ k1;

    k2 = (k2 * C2) & MASK_64;
    k2 = rotateLeft64(k2, 33n);
    k2 = (k2 * C1) & MASK_64;
    h2 = h2 ^ k2;
  }

  // Finalization mix.
  const len = BigInt(data.length);
  h1 = h1 ^ len;
  h2 = h2 ^ len;

  h1 = (h1 + h2) & MASK_64;
  h2 = (h2 + h1) & MASK_64;

  h1 = finalizationMix(h1);
  h2 = finalizationMix(h2);

  h1 = (h1 + h2) & MASK_64;
  h2 = (h2 + h1) & MASK_64;

  return [h1, h2];
}

/**
 * hashString implements the string-hashing function used in AnTS.
 */
export function hashString(s: string): string {
  const data = new TextEncoder().encode(s);
  const [hi, lo] = murmurHash128(data, 0);

  // Prepare result.
  const result = new Uint8Array(16);
  const view = new DataView(result.buffer);
  view.setBigUint64(0, hi, true);
  view.setBigUint64(8, lo, true);

  return Array.from(result)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}
