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

import { Box, Link, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

interface BuilderListDisplayProps {
  readonly groupedBuilders: {
    readonly [x: string]: readonly BuilderID[];
  };
  readonly isLoading: boolean;
}

export function BuilderListDisplay({
  groupedBuilders,
  isLoading,
}: BuilderListDisplayProps) {
  return (
    <>
      {Object.entries(groupedBuilders).map(([bucketId, builders]) => (
        <Box key={bucketId} data-testid="builder-data">
          <Typography variant="h6">{bucketId}</Typography>
          <ul>
            {builders?.map((builder) => (
              <li key={builder.builder}>
                <Link component={RouterLink} to={getBuilderURLPath(builder)}>
                  {builder.builder}
                </Link>
              </li>
            ))}
          </ul>
        </Box>
      ))}
      {isLoading && (
        <Typography component="span">
          Loading <DotSpinner />
        </Typography>
      )}
    </>
  );
}
