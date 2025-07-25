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

import { Link, Tooltip, Typography } from '@mui/material';
import { useEffect, useRef, useState } from 'react';

import { getGitilesRepoURL } from '@/gitiles/tools/utils';

export interface VersionInfoProps {
  version?: string;
}

export function VersionInfo({ version = UI_VERSION }: VersionInfoProps) {
  const versionRepoUrl = getGitilesRepoURL({
    host: 'chromium.googlesource.com',
    project: 'infra/luci/luci-go',
  });
  const href = `${versionRepoUrl}/+log/${version.split('-')[1]}`;

  const textRef = useRef<HTMLAnchorElement | null>(null);
  const [isOverflowing, setIsOverflowing] = useState(false);

  useEffect(() => {
    const textElement = textRef.current;
    if (textElement) {
      setIsOverflowing(textElement.scrollWidth > textElement.clientWidth);
    }
  }, [version]);

  const link = (
    <Link
      ref={textRef}
      href={href}
      target="_blank"
      rel="noopener"
      sx={{
        textOverflow: 'ellipsis',
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        maxWidth: '150px',
        display: 'inline-block',
        verticalAlign: 'middle',
      }}
    >
      {version}
    </Link>
  );

  return (
    <Typography
      sx={{
        p: '8px 16px',
        color: 'var(--mui-palette-text-secondary)',
        fontSize: '0.8rem',
      }}
    >
      Version:{' '}
      {isOverflowing ? (
        <Tooltip title={version} arrow>
          {link}
        </Tooltip>
      ) : (
        link
      )}
    </Typography>
  );
}
