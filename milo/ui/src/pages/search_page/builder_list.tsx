// Copyright 2022 The LUCI Authors.
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

import { Box, Typography } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';

import '@/generic_libs/components/dot_spinner';
import { useStore } from '@/common/store';
import { getBuilderURLPath } from '@/common/tools/url_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';

export const BuilderList = observer(() => {
  const { groupedBuilders, builderLoader } = useStore().searchPage;

  useEffect(() => {
    if (builderLoader) {
      builderLoader.loadRemainingPages();
    }
  }, [builderLoader]);

  return (
    <>
      {Object.entries(groupedBuilders).map(([bucketId, builders]) => (
        <Box key={bucketId}>
          <Typography variant="h6">{bucketId}</Typography>
          <ul>
            {builders?.map((builder) => (
              <li key={builder.builder}>
                <a href={getBuilderURLPath(builder)}>{builder.builder}</a>
              </li>
            ))}
          </ul>
        </Box>
      ))}
      {(builderLoader?.isLoading ?? true) && (
        <Typography component="span">
          Loading <DotSpinner />
        </Typography>
      )}
    </>
  );
});
