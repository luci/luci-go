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

import { assertNonNullable } from '@/generic_libs/tools/utils';
import {
  TestCulprit,
  TestNthSectionAnalysisResult,
  TestSingleRerun,
  TestSuspectVerificationDetails,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import {
  AnalysisStatus,
  RerunStatus,
  SingleRerun,
  SuspectVerificationDetails,
  suspectVerificationStatusToJSON,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import {
  Culprit,
  CulpritAction,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/culprits.pb';
import { GenAiSuspect } from '@/proto/go.chromium.org/luci/bisection/proto/v1/genai.pb';
import {
  NthSectionAnalysisResult,
  NthSectionSuspect,
  RegressionRange,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/nthsection.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface GenericSingleRerun {
  readonly startTime?: string;
  readonly endTime?: string;
  readonly bbid: string;
  readonly rerunResult: {
    readonly rerunStatus: RerunStatus;
  };
  readonly commit: GitilesCommit;
  readonly index: string;
  readonly type: string;
}

export const GenericSingleRerun = {
  from(rerun: SingleRerun): GenericSingleRerun {
    return {
      startTime: rerun.startTime,
      endTime: rerun.endTime,
      bbid: rerun.bbid,
      rerunResult: assertNonNullable(rerun.rerunResult),
      commit: assertNonNullable(rerun.commit),
      index: rerun.index,
      type: rerun.type,
    };
  },
  fromTest(rerun: TestSingleRerun): GenericSingleRerun {
    return {
      startTime: rerun.startTime,
      endTime: rerun.endTime,
      bbid: rerun.bbid,
      rerunResult: assertNonNullable(rerun.rerunResult),
      commit: assertNonNullable(rerun.commit),
      index: rerun.index,
      // Test analysis rpc doesn't return the rerun type because all rerun are
      // "NthSection" type.
      type: 'NthSection',
    };
  },
};

export interface GenericSuspectVerificationDetails {
  readonly status: string;
  readonly suspectRerun?: GenericSingleRerun;
  readonly parentRerun?: GenericSingleRerun;
}

const GenericSuspectVerificationDetails = {
  from(details: SuspectVerificationDetails): GenericSuspectVerificationDetails {
    return {
      status: details.status,
      suspectRerun:
        details.suspectRerun && GenericSingleRerun.from(details.suspectRerun),
      parentRerun:
        details.parentRerun && GenericSingleRerun.from(details.parentRerun),
    };
  },
  fromTest(
    details: TestSuspectVerificationDetails,
  ): GenericSuspectVerificationDetails {
    return {
      status: suspectVerificationStatusToJSON(details.status),
      suspectRerun:
        details.suspectRerun &&
        GenericSingleRerun.fromTest(details.suspectRerun),
      parentRerun:
        details.parentRerun && GenericSingleRerun.fromTest(details.parentRerun),
    };
  },
};

export interface GenericCulprit {
  readonly commit: GitilesCommit;
  readonly reviewUrl: string;
  readonly reviewTitle: string;
  readonly culpritAction: readonly CulpritAction[];
}

export const GenericCulprit = {
  from(culprit: Culprit): GenericCulprit {
    return {
      commit: assertNonNullable(culprit.commit),
      reviewUrl: culprit.reviewUrl,
      reviewTitle: culprit.reviewTitle,
      culpritAction: culprit.culpritAction,
    };
  },
  fromTest(culprit: TestCulprit): GenericCulprit {
    return {
      commit: assertNonNullable(culprit.commit),
      reviewUrl: culprit.reviewUrl,
      reviewTitle: culprit.reviewTitle,
      culpritAction: culprit.culpritAction,
    };
  },
};

export interface GenericCulpritWithDetails extends GenericCulprit {
  readonly verificationDetails: GenericSuspectVerificationDetails;
}

export const GenericCulpritWithDetails = {
  from(culprit: Culprit): GenericCulpritWithDetails {
    return {
      ...GenericCulprit.from(culprit),
      verificationDetails: GenericSuspectVerificationDetails.from(
        assertNonNullable(culprit.verificationDetails),
      ),
    };
  },
  fromTest(culprit: TestCulprit): GenericCulpritWithDetails {
    return {
      ...GenericCulprit.fromTest(culprit),
      verificationDetails: GenericSuspectVerificationDetails.fromTest(
        assertNonNullable(culprit.verificationDetails),
      ),
    };
  },
};

export interface GenericSuspect {
  readonly type: string;
  readonly reviewUrl: string;
  readonly reviewTitle: string;
  readonly verificationDetails: GenericSuspectVerificationDetails;
  readonly commit: GitilesCommit;
}

export const GenericSuspect = {
  fromGenAi(suspect: GenAiSuspect): GenericSuspect {
    return {
      type: 'AI Analysis',
      reviewUrl: suspect.reviewUrl,
      reviewTitle: suspect.reviewTitle,
      verificationDetails: GenericSuspectVerificationDetails.from(
        assertNonNullable(suspect.verificationDetails),
      ),
      commit: assertNonNullable(suspect.commit),
    };
  },
  fromNthSection(suspect: NthSectionSuspect): GenericSuspect {
    return {
      type: 'NthSection',
      reviewUrl: suspect.reviewUrl,
      reviewTitle: suspect.reviewTitle,
      verificationDetails: GenericSuspectVerificationDetails.from(
        assertNonNullable(suspect.verificationDetails),
      ),
      commit: assertNonNullable(suspect.commit),
    };
  },
  fromTestCulprit(culprit: TestCulprit): GenericSuspect {
    return {
      type: 'NthSection',
      reviewUrl: culprit.reviewUrl,
      reviewTitle: culprit.reviewTitle,
      verificationDetails: GenericSuspectVerificationDetails.fromTest(
        assertNonNullable(culprit.verificationDetails),
      ),
      commit: assertNonNullable(culprit.commit),
    };
  },
};

export interface GenericNthSectionAnalysisResult {
  readonly status: AnalysisStatus;
  readonly startTime?: string;
  readonly endTime?: string;
  readonly remainingNthSectionRange?: RegressionRange;
  readonly reruns: readonly GenericSingleRerun[];
  readonly suspect?: GenericSuspect;
}

export const GenericNthSectionAnalysisResult = {
  from(result: NthSectionAnalysisResult): GenericNthSectionAnalysisResult {
    return {
      status: result.status,
      startTime: result.startTime,
      endTime: result.endTime,
      remainingNthSectionRange: result.remainingNthSectionRange,
      reruns: result.reruns.map(GenericSingleRerun.from),
      suspect: result.suspect && GenericSuspect.fromNthSection(result.suspect),
    };
  },
  fromTest(
    result: TestNthSectionAnalysisResult,
  ): GenericNthSectionAnalysisResult {
    return {
      status: result.status,
      startTime: result.startTime,
      endTime: result.endTime,
      remainingNthSectionRange: result.remainingNthSectionRange,
      reruns: result.reruns.map(GenericSingleRerun.fromTest),
      suspect: result.suspect && GenericSuspect.fromTestCulprit(result.suspect),
    };
  },
};
