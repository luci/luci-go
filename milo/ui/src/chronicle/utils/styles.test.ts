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

import { CheckResultStatus, StageResultStatus } from './check_utils';
import { COLORS, getCheckColors, getStageColors } from './styles';

describe('styles', () => {
  describe('getCheckColors', () => {
    it('returns correct colors for CheckResultStatus', () => {
      expect(getCheckColors(CheckResultStatus.SUCCESS)).toEqual(
        COLORS.checkSuccess,
      );
      expect(getCheckColors(CheckResultStatus.FAILURE)).toEqual(
        COLORS.checkFailure,
      );
      expect(getCheckColors(CheckResultStatus.MIXED)).toEqual(
        COLORS.checkMixed,
      );
      expect(getCheckColors(CheckResultStatus.UNKNOWN)).toEqual(
        COLORS.checkPending,
      );
      expect(getCheckColors(undefined)).toEqual(COLORS.checkPending);
    });
  });

  describe('getStageColors', () => {
    it('returns correct colors for StageResultStatus', () => {
      expect(getStageColors(StageResultStatus.SUCCESS)).toEqual(
        COLORS.stageSuccess,
      );
      expect(getStageColors(StageResultStatus.FAILURE)).toEqual(
        COLORS.stageFailure,
      );
      expect(getStageColors(StageResultStatus.RUNNING)).toEqual(
        COLORS.stageRunning,
      );
      expect(getStageColors(StageResultStatus.CANCELLED)).toEqual(
        COLORS.stageCanceled,
      );
      expect(getStageColors(StageResultStatus.PENDING)).toEqual(
        COLORS.stagePending,
      );
      expect(getStageColors(StageResultStatus.UNKNOWN)).toEqual(
        COLORS.stagePending,
      );
      expect(getStageColors(undefined)).toEqual(COLORS.stagePending);
    });
  });
});
