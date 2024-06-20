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

import { NavigateNext } from '@mui/icons-material';
import { Breadcrumbs, Stack, SxProps, Theme, Typography } from '@mui/material';

import { OutputBuild } from '@/build/types';

import { AncestorBuild } from './ancestor_build';
import { BuildPathContextProvider } from './provider';

export interface AncestorBuildPathProps {
  readonly build: OutputBuild;
  readonly sx?: SxProps<Theme>;
}

export function AncestorBuildPath({ build, sx }: AncestorBuildPathProps) {
  return (
    <BuildPathContextProvider maxBatchSize={100}>
      <Stack spacing={2} sx={sx}>
        <Breadcrumbs separator={<NavigateNext fontSize="small" />}>
          {build.ancestorIds.map((bid, i) => (
            <AncestorBuild key={i} buildId={bid} />
          ))}
          <Typography sx={{ opacity: 0.8 }}>This Build</Typography>
        </Breadcrumbs>
      </Stack>
    </BuildPathContextProvider>
  );
}
