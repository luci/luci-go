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

import { fromString } from './from_string';

describe('chronicle/utils/id/from_string', () => {
  it('parses simple WorkPlan', () => {
    const parsed = fromString('Lmy-workplan');
    expect(parsed.workPlan).toEqual({ id: 'my-workplan' });
  });

  it('parses Check on WorkPlan', () => {
    const parsed = fromString('Lmy-workplan:Cmy-check');
    expect(parsed.check).toEqual({
      id: 'my-check',
      workPlan: { id: 'my-workplan' },
    });
  });

  it('parses Stage on WorkPlan', () => {
    const parsed = fromString('Lmy-workplan:Smy-stage');
    expect(parsed.stage).toEqual({
      id: 'my-stage',
      workPlan: { id: 'my-workplan' },
      isWorknode: false,
    });
  });

  it('parses Worknode Stage on WorkPlan', () => {
    const parsed = fromString('Lmy-workplan:Nmy-stage');
    expect(parsed.stage).toEqual({
      id: 'my-stage',
      workPlan: { id: 'my-workplan' },
      isWorknode: true,
    });
  });

  it('parses StageAttempt correctly', () => {
    const parsed = fromString('Lwp:Sst:A123');
    expect(parsed.stageAttempt).toEqual({
      stage: {
        id: 'st',
        workPlan: { id: 'wp' },
        isWorknode: false,
      },
      idx: 123,
    });
  });

  it('returns empty object for empty string', () => {
    expect(fromString('')).toEqual({});
  });

  describe('error conditions', () => {
    it('rejects empty tokens in the middle', () => {
      expect(() => fromString('Lwp::Cchk')).toThrow(
        'token 1 in "Lwp::Cchk" was empty?',
      );
    });

    it('rejects empty tokens at the end', () => {
      expect(() => fromString('Lwp:Cchk:')).toThrow(
        'token 2 in "Lwp:Cchk:" was empty?',
      );
    });

    it('rejects invalid non-integer index', () => {
      expect(() => fromString('Lwp:Cchk:Rabc')).toThrow(
        'expected a positive integer',
      );
      expect(() => fromString('Lwp:Cchk:R123a')).toThrow(
        'expected a positive integer',
      );
    });

    it('rejects negative index', () => {
      expect(() => fromString('Lwp:Cchk:R-1')).toThrow('bad idx: must be > 0');
    });

    it('rejects zero index', () => {
      expect(() => fromString('Lwp:Cchk:R0')).toThrow('bad idx: must be > 0');
    });

    it('rejects malformed timestamps', () => {
      expect(() => fromString('Lwp:Cchk:Vbadts')).toThrow(
        'bad timestamp: needed seconds/nanos',
      );
    });

    it('rejects invalid seconds inside timestamp', () => {
      expect(() => fromString('Lwp:Cchk:Vabc/123')).toThrow(
        'bad timestamp: parsing seconds "abc"',
      );
      expect(() => fromString('Lwp:Cchk:V123a/123')).toThrow(
        'bad timestamp: parsing seconds "123a"',
      );
    });

    it('rejects invalid nanos inside timestamp', () => {
      expect(() => fromString('Lwp:Cchk:V123/abc')).toThrow(
        'bad timestamp: parsing nanos "abc"',
      );
    });

    it('rejects unexpected keys', () => {
      expect(() => fromString('Lwp1:Lwp2')).toThrow("unexpected key 'L'");
    });
  });
});
