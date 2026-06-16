// Copyright 2023 The LUCI Authors.
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
  Alert,
  AlertTitle,
  Box,
  CircularProgress,
  FormControl,
  InputLabel,
  Link,
  MenuItem,
  Select,
} from '@mui/material';
import { useId, useState } from 'react';
import { Link as RouterLink } from 'react-router';

import { GenericSuspect } from '@/bisection/types';
import { getBlamelistPins } from '@/build/tools/build_utils';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs';
import { getGitilesRepoURL } from '@/gitiles/tools/utils';

import { useAnalysis, useAnalysisError, useBuild } from '../context';

import { BlamelistDisplay } from './blamelist_display';

export function BlamelistTab() {
  const build = useBuild();
  const analysis = useAnalysis();
  const analysisError = useAnalysisError();

  const repoSelectorLabelId = useId();

  const [selectedBlamelistPinIndex, setSelectedBlamelistPinIndex] = useState(0);

  if (!build) {
    return <CircularProgress sx={{ margin: '10px' }} />;
  }

  const blamelistPins = getBlamelistPins(build);
  const selectedBlamelistPin = blamelistPins[selectedBlamelistPinIndex];

  if (!selectedBlamelistPin) {
    return (
      <div css={{ padding: '10px' }}>
        Blamelist is not available because the build has no associated gitiles
        commit.
      </div>
    );
  }

  const project = build.builder.project;
  const bbid = build.id;

  let suspect: GenericSuspect | undefined;
  if (
    analysis?.genAiResult?.suspect?.verificationDetails?.status ===
    'Confirmed Culprit'
  ) {
    suspect = analysis?.genAiResult?.suspect
      ? GenericSuspect.fromGenAi(analysis?.genAiResult?.suspect)
      : undefined;
  } else if (
    analysis?.nthSectionResult?.suspect?.verificationDetails?.status ===
    'Confirmed Culprit'
  ) {
    suspect = analysis?.nthSectionResult?.suspect
      ? GenericSuspect.fromNthSection(analysis?.nthSectionResult?.suspect)
      : undefined;
  } else {
    suspect = analysis?.genAiResult?.suspect
      ? GenericSuspect.fromGenAi(analysis?.genAiResult?.suspect)
      : undefined;
  }

  return (
    <>
      {analysisError && (
        <Alert severity="warning" sx={{ margin: '10px' }}>
          Failed to load secondary data. If you log in you may be able to see
          more information on this page.
        </Alert>
      )}
      {suspect && (
        <Alert
          severity={
            suspect.verificationDetails.status === 'Confirmed Culprit'
              ? 'error'
              : 'warning'
          }
          sx={{ margin: '10px', border: '1px solid' }}
        >
          <AlertTitle sx={{ fontWeight: 'bold' }}>
            {suspect.verificationDetails.status === 'Confirmed Culprit'
              ? 'Confirmed Culprit Identified'
              : 'Suspected Culprit Identified'}
          </AlertTitle>
          This build failure has been analyzed by{' '}
          <strong>{suspect.type}</strong>. The suspected change is:
          <Box sx={{ mt: 1, mb: 1 }}>
            <Link
              href={suspect.reviewUrl}
              target="_blank"
              rel="noopener noreferrer"
              sx={{ fontWeight: 'bold' }}
            >
              {suspect.commit.id.slice(0, 10)}
            </Link>{' '}
            - <em>{suspect.reviewTitle}</em>
          </Box>
          Verification status:{' '}
          <strong>{suspect.verificationDetails.status}</strong>.
          {bbid && (
            <Box sx={{ mt: 1 }}>
              <Link
                component={RouterLink}
                to={`/ui/p/${project}/bisection/compile-analysis/b/${bbid}`}
                sx={{ textDecoration: 'underline', fontWeight: 'bold' }}
              >
                View Full Analysis Page
              </Link>
            </Box>
          )}
        </Alert>
      )}
      <FormControl size="small" sx={{ margin: '10px' }}>
        <InputLabel id={repoSelectorLabelId}>Repo</InputLabel>
        <Select
          labelId={repoSelectorLabelId}
          label="Repo"
          value={selectedBlamelistPinIndex}
          disabled={blamelistPins.length <= 1}
          onChange={(e) =>
            setSelectedBlamelistPinIndex(e.target.value as number)
          }
          sx={{ width: '500px' }}
        >
          {blamelistPins.map((pin, i) => (
            <MenuItem key={i} value={i}>
              {getGitilesRepoURL(pin)}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <BlamelistDisplay
        blamelistPin={selectedBlamelistPin}
        builder={build.builder}
        analysis={analysis}
      />
    </>
  );
}

export function Component() {
  useDeclareTabId('blamelist');

  return (
    <TrackLeafRoutePageView contentGroup="blamelist">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="blamelist"
      >
        <BlamelistTab />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
