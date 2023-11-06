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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { AccordionDetails, Typography } from '@mui/material';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import { useState } from 'react';

import { TestResult } from '@/common/services/resultdb';

interface Props {
  result: TestResult;
}

export function ResultBasicInfo({ result }: Props) {
  const [expanded, setExpanded] = useState(true);

  return (
    <Accordion
      variant="outlined"
      disableGutters
      expanded={expanded}
      onChange={() => setExpanded(!expanded)}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Grid item container columnGap={1} alignItems="center">
          {expanded || !result.failureReason ? (
            <Typography>Details</Typography>
          ) : (
            <Grid
              className="failure-bg"
              item
              sx={{
                p: 1,
              }}
            >
              {result.failureReason.primaryErrorMessage}
            </Grid>
          )}
        </Grid>
      </AccordionSummary>
      <AccordionDetails>
        <Grid container rowGap={2}>
          <Grid item container columnGap={1} alignItems="center">
            {result.duration && (
              <>
                <Chip label={result.duration} />
                <Divider orientation="vertical" />
              </>
            )}
            {/** TODO(b/308716449): Display swarming task when available. */}
            Swarming Task:
            <Link href="#">626a273fe4f74c11</Link>
          </Grid>
          {result.failureReason && (
            <Grid item container columnGap={1} alignItems="center">
              <Grid item>
                Failure reason (<Link href="#">similar failures</Link>):
              </Grid>
              <Grid
                className="failure-bg"
                item
                sx={{
                  p: 1,
                }}
              >
                {result.failureReason.primaryErrorMessage}
              </Grid>
            </Grid>
          )}
          {/** TODO(b/308716449): Display related bugs. */}
          <Grid item container columnGap={1}>
            Related bugs:
            <Link href="#">b/123456789</Link>
            <Link href="#">b/123456789</Link>
            <Link href="#">b/123456789</Link>
          </Grid>
          {/** TODO(b/308716449): Display last successful patchset of the CL. */}
          <Grid item container columnGap={1}>
            Last successful CV run:
            <Link href="#">PATCHSET 3 - Attempt 2</Link>
          </Grid>
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}
