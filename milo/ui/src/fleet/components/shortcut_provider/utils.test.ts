// Copyright 2026 The LUCI Authors.
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

import { parseShortcut, KeyChord, formatShortcut } from './utils';

describe('parseShortcut', () => {
  it('parses single key', () => {
    const result = parseShortcut('s');
    expect(result).toHaveLength(1);
    expect(result[0]).toEqual(
      expect.objectContaining({ key: 's', ctrl: false, meta: false }),
    );
  });

  it('parses modifiers', () => {
    const result = parseShortcut('Control+s');
    expect(result).toHaveLength(1);
    expect(result[0]).toEqual(
      expect.objectContaining({ key: 's', ctrl: true, meta: false }),
    );
  });

  it('parses sequences', () => {
    const result = parseShortcut('g h');
    expect(result).toHaveLength(2);
    expect(result[0].key).toBe('g');
    expect(result[1].key).toBe('h');
  });

  it('parses sequences with modifiers', () => {
    const result = parseShortcut('Control+k s');
    expect(result).toHaveLength(2);
    expect(result[0]).toEqual(
      expect.objectContaining({ key: 'k', ctrl: true }),
    );
    expect(result[1]).toEqual(
      expect.objectContaining({ key: 's', ctrl: false }),
    );
  });

  it('normalizes keys', () => {
    const result = parseShortcut('Esc');
    expect(result[0].key).toBe('Escape');
  });
});

describe('formatShortcut', () => {
  it('formats single key', () => {
    const seq: KeyChord[] = [
      { key: 's', ctrl: false, meta: false, alt: false, shift: false },
    ];
    expect(formatShortcut(seq)).toEqual([['s']]);
  });

  it('formats modifiers', () => {
    const seq: KeyChord[] = [
      { key: 's', ctrl: true, meta: false, alt: false, shift: false },
    ];
    expect(formatShortcut(seq)).toEqual([['Ctrl', 's']]);
  });

  it('formats sequence', () => {
    const seq: KeyChord[] = [
      { key: 'g', ctrl: false, meta: false, alt: false, shift: false },
      { key: 'h', ctrl: false, meta: false, alt: false, shift: false },
    ];
    expect(formatShortcut(seq)).toEqual([['g'], ['h']]);
  });
});
