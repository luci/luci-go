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

import {
  Box,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableRow,
  Typography,
} from '@mui/material';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ArtifactContentHeader } from './artifact_content_header';

interface DefaultArtifactContentProps {
  artifact: Artifact;
  contentType: string;
}

export function DefaultArtifactContent({
  artifact,
  contentType,
}: DefaultArtifactContentProps) {
  const sizeBytes = artifact.sizeBytes ? Number(artifact.sizeBytes) : null;
  const formattedSize =
    sizeBytes !== null
      ? sizeBytes > 1024 * 1024
        ? `${(sizeBytes / 1024 / 1024).toFixed(2)} MB`
        : sizeBytes > 1024
          ? `${(sizeBytes / 1024).toFixed(2)} KB`
          : `${sizeBytes} B`
      : 'Unknown';

  return (
    <>
      <ArtifactContentHeader
        artifact={artifact}
        contentData={{
          contentType: contentType,
        }}
        showMetadata={false}
      />
      <Box sx={{ p: 2 }}>
        <Typography color="text.secondary" paragraph>
          Preview not available for content type: {contentType}.
        </Typography>
        <TableContainer component={Paper} elevation={1} sx={{ maxWidth: 800 }}>
          <Table size="small">
            <TableBody>
              <TableRow>
                <TableCell
                  component="th"
                  scope="row"
                  sx={{ width: 150, fontWeight: 'bold' }}
                >
                  Name
                </TableCell>
                <TableCell>{artifact.artifactId}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell
                  component="th"
                  scope="row"
                  sx={{ width: 150, fontWeight: 'bold' }}
                >
                  Size
                </TableCell>
                <TableCell>{formattedSize}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell
                  component="th"
                  scope="row"
                  sx={{ width: 150, fontWeight: 'bold' }}
                >
                  Content Type
                </TableCell>
                <TableCell>{contentType}</TableCell>
              </TableRow>
              {artifact.fetchUrl && (
                <TableRow>
                  <TableCell
                    component="th"
                    scope="row"
                    sx={{ width: 150, fontWeight: 'bold' }}
                  >
                    Raw Artifact
                  </TableCell>
                  <TableCell>
                    <Link
                      href={artifact.fetchUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      underline="hover"
                    >
                      Open Raw Artifact
                    </Link>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    </>
  );
}
