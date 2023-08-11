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

import { useStore } from '@/common/store';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import { RelatedBuildTable } from './related_build_table';

export const RelatedBuildsTab = observer(() => {
  useTabId('related-builds');
  const store = useStore();

  if (!store.buildPage.build || !store.buildPage.relatedBuilds) {
    return (
      <Box sx={{ p: 1, color: 'var(--active-text-color' }}>
        Loading <DotSpinner />
      </Box>
    );
  }

  if (!store.buildPage.relatedBuilds.length) {
    return (
      <Box sx={{ p: 1 }}>No other builds found with the same buildset</Box>
    );
  }

  return (
    <Box>
      <Box sx={{ p: 2 }}>
        <Typography variant="h6">
          Other builds with the same buildset
        </Typography>
        <ul>
          {store.buildPage.build.buildSets.map((bs) => (
            <li key={bs}>{bs}</li>
          ))}
        </ul>
      </Box>
      <RelatedBuildTable relatedBuilds={store.buildPage.relatedBuilds} />
    </Box>
  );
});
