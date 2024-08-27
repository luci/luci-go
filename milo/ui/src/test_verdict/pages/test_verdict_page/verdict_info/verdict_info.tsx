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

import { Typography } from '@mui/material';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import { Fragment, useState } from 'react';

import {
  generateTestHistoryURLSearchParams,
  getCodeSourceUrl,
  getTestHistoryURLWithSearchParam,
} from '@/common/tools/url_utils';
import { getSortedTestVariantDef } from '@/test_verdict/tools/utils';

import { useProject, useTestVerdict } from '../context';

export function VerdictInfo() {
  const verdict = useTestVerdict();
  const project = useProject();

  const [drawerOpen, setDrawerOpen] = useState(false);
  function toggleDrawer(open: boolean) {
    setDrawerOpen(open);
  }

  if (!project) {
    throw new Error('Invariant violated: a project must be selected.');
  }

  return (
    <Grid
      item
      container
      columnGap={1}
      alignItems="center"
      sx={{ ml: 0.5, mb: 0.5 }}
    >
      {verdict.variant && (
        <>
          <Typography fontSize="0.9rem">
            {getSortedTestVariantDef(verdict.variant.def || {}).map(
              (entry, i) => (
                <Fragment key={entry[0]}>
                  {i > 0 && ', '}
                  {entry[0]}: {entry[1]}
                </Fragment>
              ),
            )}
          </Typography>
          <Divider orientation="vertical" flexItem />
          <Link
            target="_blank"
            href={getTestHistoryURLWithSearchParam(
              project,
              verdict.testId,
              generateTestHistoryURLSearchParams(verdict.variant),
            )}
          >
            History
          </Link>
        </>
      )}

      {verdict.testMetadata?.location && (
        <>
          {verdict.variant && <Divider orientation="vertical" flexItem />}
          <Link
            target="_blank"
            href={getCodeSourceUrl(verdict.testMetadata.location)}
          >
            Test source
          </Link>
        </>
      )}
      {/** TODO(b/308716044): Display the test properties in the drawer. */}
      {verdict.testMetadata?.properties && (
        <>
          <Divider orientation="vertical" flexItem />
          <Button
            color="primary"
            variant="outlined"
            onClick={() => toggleDrawer(true)}
            size="small"
          >
            Metadata
          </Button>
          <Drawer
            anchor="bottom"
            open={drawerOpen}
            onClose={() => toggleDrawer(false)}
            sx={{
              height: '500px',
            }}
            PaperProps={{
              sx: {
                height: '500px',
              },
            }}
          >
            <Grid
              container
              sx={{
                p: 1,
              }}
            >
              name: {verdict.testMetadata.name}
            </Grid>
          </Drawer>
        </>
      )}
    </Grid>
  );
}
