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

import { CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import * as Diff2html from 'diff2html';

import 'diff2html/bundles/css/diff2html.min.css';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

interface Props {
  artifact: Artifact;
}

export function TextDiff({ artifact }: Props) {
  const { data, isError, error, isPending } = useQuery({
    queryFn: async () => {
      const res = await fetch(
        urlSetSearchQueryParam(artifact.fetchUrl, 'n', ARTIFACT_LENGTH_LIMIT),
      );
      return res.text();
    },
    queryKey: [artifact.fetchUrl, artifact.artifactId],
  });

  if (isError) {
    throw error;
  }

  if (isPending) {
    return <CircularProgress />;
  }

  return (
    <SanitizedHtml
      html={Diff2html.html(data, {
        drawFileList: false,
      })}
    />
  );
}
