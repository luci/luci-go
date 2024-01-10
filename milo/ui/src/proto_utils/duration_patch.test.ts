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

import { Duration } from '@/proto/google/protobuf/duration.pb';

describe('Duration', () => {
  describe('JSON (de)serialization', () => {
    it('can (de)serialize positive duration', () => {
      const srcMsg = Duration.fromPartial({ seconds: '1000', nanos: 1000 });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('1000.000001s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });

    it('can (de)serialize negative duration', () => {
      const srcMsg = Duration.fromPartial({ seconds: '-1000', nanos: -1000 });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('-1000.000001s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });

    it('can (de)serialize small positive duration', () => {
      const srcMsg = Duration.fromPartial({ seconds: '0', nanos: 1000 });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('0.000001s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });

    it('can (de)serialize small negative duration', () => {
      const srcMsg = Duration.fromPartial({ seconds: '0', nanos: -1000 });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('-0.000001s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });

    it('can (de)serialize long duration', () => {
      const srcMsg = Duration.fromPartial({
        seconds: '-10000000000000',
        nanos: -1,
      });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('-10000000000000.000000001s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });

    it('can (de)serialize short duration', () => {
      const srcMsg = Duration.fromPartial({
        seconds: '1',
      });
      const json = Duration.toJSON(srcMsg);
      expect(json).toStrictEqual('1s');
      expect(Duration.fromJSON(json)).toEqual(srcMsg);
    });
  });
});
