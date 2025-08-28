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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';

import { HtmlTooltip } from '@/common/components/html_tooltip';

export const TruncatedHistoryExplanation = () => {
  return (
    <HtmlTooltip
      title={
        <>
          <p>
            Please find the first failing build in the history and view its
            blamelist.
          </p>
          <p style={{ opacity: 0.7 }}>
            Only the last 10 builds for each builder are loaded. If all 10
            builds are failing, the first failing build or blamelist cannot be
            automatically determined.
          </p>
        </>
      }
    >
      <HelpOutlineIcon
        fontSize="small"
        sx={{
          verticalAlign: 'middle',
          color: 'rgba(0, 0, 0, 0.54)',
        }}
      />
    </HtmlTooltip>
  );
};
