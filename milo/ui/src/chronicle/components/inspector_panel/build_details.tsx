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

import {
  BuildCheckOptions,
  productToJSON,
} from '@/proto/turboci/data/build/v1/build_check_options.pb';
import { BuildCheckResult } from '@/proto/turboci/data/build/v1/build_check_results.pb';

import { DetailRow } from './detail_row';

export function BuildCheckOptionsDetails({
  data,
}: {
  data: BuildCheckOptions;
}) {
  if (!data.target)
    return <Typography variant="body2">No target info</Typography>;
  return (
    <Box>
      <DetailRow label="Target Name" value={data.target.name} />
      <DetailRow label="Target Namespace" value={data.target.namespace} />
      <DetailRow
        label="Target Product"
        value={data.target.product ? productToJSON(data.target.product) : 'N/A'}
      />
      <DetailRow label="Target Platform" value={data.target.platform} />
      <DetailRow label="Target Device" value={data.target.device} />
    </Box>
  );
}

export function BuildCheckResultDetails({ data }: { data: BuildCheckResult }) {
  return (
    <Box>
      <DetailRow label="Success" value={data.success ? 'True' : 'False'} />
      <DetailRow label="Message" value={data.displayMessage?.message} />
      {data.viewUrl && (
        <DetailRow
          label="View URL"
          value={
            <Link href={data.viewUrl} target="_blank" rel="noopener">
              {data.viewUrl}
            </Link>
          }
        />
      )}
      {data.androidBuildArtifacts && (
        <Box sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}>
          <Typography variant="caption">Android Build Artifacts</Typography>
          <Box sx={{ pl: 1 }}>
            <DetailRow
              label="Build ID"
              value={data.androidBuildArtifacts.buildId}
            />
            <DetailRow
              label="Target"
              value={data.androidBuildArtifacts.target}
            />
            <DetailRow
              label="Build Attempt"
              value={data.androidBuildArtifacts.buildAttempt}
            />
          </Box>
        </Box>
      )}
      {data.casManifest && (
        <Box sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}>
          <Typography variant="caption">CAS Manifest</Typography>
          <Box sx={{ pl: 1 }}>
            <DetailRow
              label="Manifest"
              value={Object.entries(data.casManifest.manifest)
                .map(([key, value]) => `${key}: ${value}`)
                .join('\n')}
            />
            <DetailRow
              label="CAS Instance"
              value={data.casManifest.casInstance}
            />
            <DetailRow
              label="CAS Service"
              value={data.casManifest.casService}
            />
            <DetailRow
              label="Client Version"
              value={data.casManifest.clientVersion}
            />
          </Box>
        </Box>
      )}
      {Object.keys(data.gcsArtifacts).length > 0 && (
        <GcsArtifactsDetails artifacts={data.gcsArtifacts} />
      )}
    </Box>
  );
}

function GcsArtifactsDetails({
  artifacts,
}: {
  artifacts: BuildCheckResult['gcsArtifacts'];
}) {
  return (
    <Box sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}>
      <Typography variant="caption">GCS Artifacts</Typography>
      {Object.entries(artifacts).map(([key, artifact]) => (
        <Box key={key} sx={{ mt: 1, pl: 1, borderLeft: '2px solid #eee' }}>
          <Box sx={{ pl: 1 }}>
            <DetailRow label="Group" value={key} />
            <DetailRow
              label="Root Directory URI"
              value={artifact.rootDirectoryUri}
            />
            {Object.entries(artifact.filesByCategory).map(
              ([category, fileList]) => (
                <DetailRow
                  key={category}
                  label={category}
                  value={
                    <Box component="ul" sx={{ pl: 3, m: 0 }}>
                      {fileList.files.map((file, idx) => (
                        <li key={idx}>
                          <Typography
                            variant="body2"
                            component="span"
                            sx={{
                              fontFamily: 'monospace',
                              fontSize: 'inherit',
                              wordBreak: 'break-all',
                            }}
                          >
                            {file}
                          </Typography>
                        </li>
                      ))}
                    </Box>
                  }
                />
              ),
            )}
          </Box>
        </Box>
      ))}
    </Box>
  );
}
