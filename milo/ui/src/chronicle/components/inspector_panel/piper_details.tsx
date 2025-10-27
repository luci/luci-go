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

import { Box } from '@mui/material';

import { PiperSourceCheckOptions } from '@/proto/turboci/data/piper/v1/piper_source_check_options.pb';

import { DetailRow } from './detail_row';

export function PiperSourceCheckOptionsDetails({
  data,
}: {
  data: PiperSourceCheckOptions;
}) {
  return (
    <Box>
      <DetailRow label="CL Number" value={data.clNumber || 'HEAD'} />
      {data.files.length > 0 && (
        <DetailRow
          label="Files"
          value={
            <ul style={{ margin: 0, paddingLeft: 20 }}>
              {data.files.map((f, i) => (
                <li key={i}>{f}</li>
              ))}
            </ul>
          }
        />
      )}
      {data.targets.length > 0 && (
        <DetailRow
          label="Targets"
          value={
            <ul style={{ margin: 0, paddingLeft: 20 }}>
              {data.targets.map((t, i) => (
                <li key={i}>{t}</li>
              ))}
            </ul>
          }
        />
      )}
    </Box>
  );
}
