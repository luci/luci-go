// Copyright 2026 The LUCI Authors.
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

import PestControlOutlinedIcon from '@mui/icons-material/PestControlOutlined';
import Tooltip from '@mui/material/Tooltip';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

interface Props {
  name: string | string[];
  project?: string;
}

export const BuganizerLink = ({ name, project }: Props) => {
  const { trackEvent } = useGoogleAnalytics();

  const query = Array.isArray(name) ? name.join(' OR ') : name;
  const href = `http://b/${encodeURIComponent(query)}`;

  return (
    <Tooltip title="Search for bugs related to this device in Buganizer">
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        style={{ display: 'flex', alignItems: 'center' }}
        onClick={() =>
          trackEvent('buganizer_link_dut_clicked', {
            componentName: 'BuganizerLink',
            project,
          })
        }
      >
        <PestControlOutlinedIcon fontSize="small" />
      </a>
    </Tooltip>
  );
};
