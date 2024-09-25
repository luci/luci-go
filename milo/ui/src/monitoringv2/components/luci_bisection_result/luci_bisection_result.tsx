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

import Box from '@mui/material/Box';
import Link from '@mui/material/Link';

import { LuciBisectionResult } from '@/monitoringv2/util/server_json';

import { CulpritSection } from './culprit_section';
import { HeuristicResult } from './heuristic_result';
import { NthSectionResult } from './nth_section_result';
import { generateAnalysisUrl } from './util';

interface LuciBisectionResultSectionProps {
  result: LuciBisectionResult | null;
}

export const LuciBisectionResultSection = ({
  result,
}: LuciBisectionResultSectionProps) => {
  if (result === null) {
    return <></>;
  }

  if (!result.is_supported) {
    return <>LUCI Bisection does not support the failure in this builder.</>;
  }

  if (!result.analysis || !result.failed_bbid) {
    return <>LUCI Bisection couldn&apos;t find an analysis for this failure.</>;
  }

  // If there is a culprit, display it
  const culprits = result.analysis.culprits ?? [];
  if (culprits.length > 0) {
    return (
      <CulpritSection
        culprits={culprits}
        failed_bbid={result.failed_bbid}
      ></CulpritSection>
    );
  }

  return (
    <>
      <Link
        href={generateAnalysisUrl(result.failed_bbid)}
        target="_blank"
        rel="noopener"
      >
        LUCI Bisection
      </Link>
      &nbsp; results
      {result.analysis.nth_section_result ? (
        <Box sx={{ margin: '5px' }}>
          <NthSectionResult
            nth_section_result={result.analysis.nth_section_result}
          ></NthSectionResult>
        </Box>
      ) : null}
      {result.analysis.heuristic_result ? (
        <Box sx={{ margin: '5px' }}>
          <HeuristicResult
            heuristicResult={result.analysis.heuristic_result}
          ></HeuristicResult>
        </Box>
      ) : null}
    </>
  );
};
