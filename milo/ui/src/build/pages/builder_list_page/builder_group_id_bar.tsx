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

import { Link } from '@mui/material';

import { getProjectURLPath } from '@/common/tools/url_utils';

export interface ProjectIdBarProps {
  readonly project: string;
}

export function BuilderGroupIdBar({ project }: ProjectIdBarProps) {
  return (
    <div
      css={{
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        display: 'flex',
      }}
    >
      <div css={{ flex: '0 auto' }}>
        <Link href={getProjectURLPath(project)}>{project}</Link>
        <span> / </span>
        <span>builders</span>
      </div>
    </div>
  );
}
