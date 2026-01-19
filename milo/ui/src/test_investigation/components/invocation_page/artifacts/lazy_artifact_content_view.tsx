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

import { Box, CircularProgress, Typography } from '@mui/material';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { ArtifactContentView } from '@/test_investigation/components/common/artifacts/content/artifact_content_view';
import { useLazyArtifact } from '@/test_investigation/hooks/use_lazy_artifact';

interface LazyArtifactContentViewProps {
  artifact: Artifact;
}

export function LazyArtifactContentView({
  artifact,
}: LazyArtifactContentViewProps) {
  const { data: fullArtifact, isLoading, error } = useLazyArtifact(artifact);

  if (isLoading) {
    return (
      <Box
        sx={{
          p: 2,
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100%',
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography color="error">
          Failed to load artifact details: {error.message}
        </Typography>
      </Box>
    );
  }

  if (!fullArtifact) {
    return null;
  }

  return (
    <ArtifactContentView artifact={fullArtifact} enableLogComparison={false} />
  );
}
