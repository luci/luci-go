// Copyright 2023 The LUCI Authors.
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

import Link from '@mui/material/Link';
import { JSONPath as jsonpath } from 'jsonpath-plus';

import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { StringPair } from '@/common/services/common';
import { MiloInternal, Project } from '@/common/services/milo_internal';
import { TestMetadata } from '@/common/services/resultdb';
import { getCodeSourceUrl } from '@/common/tools/url_utils';
import { extractProject } from '@/common/tools/utils';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';

import { useTestMetadata } from './utils';

export interface TestIdLabelProps {
  readonly projectOrRealm: string; // A project name or a realm name.
  readonly testId: string;
}

function propertiesToDisplay(
  projectCfg: Project,
  testMetadata: TestMetadata,
): StringPair[] {
  const propertiesCfgs = projectCfg.metadataConfig?.testMetadataProperties;
  // Return empty if no test metadata properties or, test metadata properties schema or display config is unspecified.
  if (
    !testMetadata.properties ||
    !testMetadata.propertiesSchema ||
    !propertiesCfgs
  ) {
    return [];
  }
  // Find the right display rules to apply to the test metadata properties based on the schema name.
  const propertiesCfg = propertiesCfgs.find(
    (t) => t.schema === testMetadata.propertiesSchema,
  );
  if (!propertiesCfg) {
    return [];
  }
  const pairs: StringPair[] = [];
  for (const item of propertiesCfg.displayItems) {
    const value = jsonpath<string | string[] | undefined>({
      json: testMetadata.properties,
      path: `$.${item.path}@string()`,
      wrap: false,
    });
    if (value && typeof value === 'string') {
      pairs.push({ key: item.displayName, value });
    }
  }
  return pairs;
}

export function TestIdLabel({ projectOrRealm, testId }: TestIdLabelProps) {
  const project = extractProject(projectOrRealm);
  const {
    data: testMetadataDetail,
    isSuccess: tmIsSuccess,
    isLoading: tmIsLoading,
  } = useTestMetadata({ project, predicate: { testIds: [testId] } });
  const metadata = testMetadataDetail?.testMetadata;
  const testLocation = metadata?.location;
  const sourceURL = testLocation
    ? getCodeSourceUrl(
        TestLocation.fromPartial(testLocation),
        testMetadataDetail?.sourceRef.gitiles?.ref,
      )
    : null;
  const {
    data: projectCfg,
    isSuccess: cfgIsSuccess,
    isLoading: cfgIsLoading,
  } = usePrpcQuery({
    host: '',
    insecure: location.protocol === 'http:',
    Service: MiloInternal,
    method: 'getProjectCfg',
    request: { project },
  });

  return (
    <table>
      <tbody>
        <tr>
          <td
            css={{
              color: 'var(--light-text-color)',
              /* Shrink the first column */
              width: '0px',
            }}
          >
            Project
          </td>
          <td>{project}</td>
        </tr>
        <tr>
          <td css={{ color: 'var(--light-text-color)' }}>ID</td>
          <td>{testId}</td>
        </tr>
        {!tmIsLoading && tmIsSuccess && metadata && (
          <>
            {metadata.name && (
              <tr>
                <td css={{ color: 'var(--light-text-color)' }}>Test</td>
                {sourceURL ? (
                  <td>
                    <Link href={sourceURL} target="_blank">
                      {metadata.name}
                    </Link>
                  </td>
                ) : (
                  <td>{metadata.name}</td>
                )}
              </tr>
            )}
            {!cfgIsLoading &&
              cfgIsSuccess &&
              propertiesToDisplay(projectCfg, metadata).map((item, idx) => (
                <tr key={idx}>
                  <td css={{ color: 'var(--light-text-color)' }}>{item.key}</td>
                  <td>{item.value}</td>
                </tr>
              ))}
          </>
        )}
      </tbody>
    </table>
  );
}
