// Copyright 2025 The LUCI Authors.
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

import { useTree } from '@/monitoringv2/pages/monitoring_page/context';

export const BugsHeader = () => {
  const tree = useTree();
  return (
    <Box sx={{ padding: '16px' }}>
      <Typography variant="h5">
        Open Monitoring Bugs
        <span style={{ fontSize: '14px', opacity: '50%', paddingLeft: '12px' }}>
          All open bugs in the{' '}
          <Link
            href={`https://b.corp.google.com/issues?q=hotlistid:${tree!.hotlistId}%20status:open`}
            target="_blank"
          >
            monitoring hotlist
          </Link>{' '}
          or associated with an alert group.
        </span>
      </Typography>
    </Box>
  );
};
