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

import { Box, Paper, Typography } from '@mui/material';
import { ReactNode } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { Sticky } from '@/generic_libs/components/queued_sticky';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

interface ArtifactContentHeaderProps {
  artifact: Artifact;
  contentData?: {
    isText?: boolean;
    data?: string | null;
    contentType?: string;
  };
  searchBox?: ReactNode;
  actions?: ReactNode;
  sticky?: boolean;
  showMetadata?: boolean;
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 Bytes';
  const sizes = ['Bytes', 'KiB', 'MiB', 'GiB', 'TiB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + sizes[i];
}

export function ArtifactContentHeader({
  artifact,
  contentData,
  searchBox,
  actions,
  sticky = false,
  showMetadata = true,
}: ArtifactContentHeaderProps) {
  const decodedArtifactId = decodeURIComponent(artifact.artifactId);
  const fileName = decodedArtifactId.split('/').pop() || decodedArtifactId;
  const formattedSize =
    artifact.sizeBytes !== null
      ? formatFileSize(Number(artifact.sizeBytes))
      : 'N/A';
  const displayContentType =
    contentData?.contentType || artifact.contentType || 'N/A';

  const headerText = (
    <Typography
      variant="h6"
      noWrap
      sx={{
        fontWeight: 'normal',
        cursor: showMetadata ? 'help' : 'default',
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        textOverflow: 'ellipsis',
        minWidth: 0,
      }}
    >
      {fileName}
    </Typography>
  );

  const content = (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        justifyContent: 'space-between',
        width: '100%',
        minWidth: 0,
      }}
    >
      <Box
        sx={{
          flex: 1,
          overflow: 'hidden',
          minWidth: 0,
        }}
      >
        {showMetadata ? (
          <HtmlTooltip
            title={
              <Box
                sx={{
                  display: 'grid',
                  gridTemplateColumns: 'auto 1fr auto',
                  gap: 1,
                  alignItems: 'center',
                }}
              >
                <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
                  Path:
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    wordBreak: 'break-all',
                    color: 'text.secondary',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                  }}
                >
                  <span>
                    <span>
                      {decodedArtifactId.substring(
                        0,
                        decodedArtifactId.lastIndexOf('/') + 1,
                      )}
                    </span>
                    <Box
                      component="span"
                      sx={{ color: 'text.primary', fontWeight: 'medium' }}
                    >
                      {decodedArtifactId.substring(
                        decodedArtifactId.lastIndexOf('/') + 1,
                      )}
                    </Box>
                  </span>
                </Typography>
                <CopyToClipboard
                  textToCopy={decodedArtifactId}
                  title="Copy full artifact path"
                  sx={{ ml: 1 }}
                />

                <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
                  Type:
                </Typography>
                <Typography variant="body2">{displayContentType}</Typography>
                <Box />

                <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
                  Size:
                </Typography>
                <Typography variant="body2">{formattedSize}</Typography>
                <Box />
              </Box>
            }
          >
            {headerText}
          </HtmlTooltip>
        ) : (
          headerText
        )}
      </Box>

      {searchBox && <Box sx={{ width: 400, flexShrink: 0 }}>{searchBox}</Box>}

      {actions && (
        <Box
          sx={{ flexShrink: 0, display: 'flex', gap: 1, alignItems: 'center' }}
        >
          {actions}
        </Box>
      )}
    </Box>
  );

  if (sticky) {
    return (
      <Sticky
        top
        sx={{
          zIndex: 100,
          backgroundColor: 'background.paper',
          borderBottom: '1px solid',
          borderColor: 'divider',
          minWidth: 0,
          boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
        }}
      >
        <Box sx={{ px: 3, py: 1.5 }}>{content}</Box>
      </Sticky>
    );
  }

  return (
    <Paper
      square
      elevation={0}
      sx={{
        borderBottom: '1px solid',
        borderColor: 'divider',
        zIndex: 1,
      }}
    >
      <Box sx={{ p: 3 }}>{content}</Box>
    </Paper>
  );
}
