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

import Alert, { AlertProps } from '@mui/material/Alert';
import Link from '@mui/material/Link';
import { useMemo } from 'react';
import { Link as RouterLink } from 'react-router';

import { useFeatureFlag } from '@/common/feature_flags';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { NEW_TEST_INVESTIGATION_PAGE_FLAG } from '@/test_investigation/pages/features';

export interface RedirectBackBannerProps extends AlertProps {
  invocation: Invocation;
  /**
   * Optional. If supplied, the redirect will deep link directly to this test variant.
   * Takes precedence over parsedTestId/parsedVariantDef.
   */
  testVariant?: TestVariant;
  /** Optional. Test ID parsed from the URL. Used if testVariant is not provided. */
  parsedTestId?: string | null;
  /** Optional. Variant definition parsed from the URL. Used if testVariant is not provided. */
  parsedVariantDef?: Readonly<Record<string, string>> | null;
}

export function RedirectBackBanner({
  invocation,
  testVariant,
  parsedTestId,
  parsedVariantDef,
  ...alertProps
}: RedirectBackBannerProps) {
  // This is here purely for the side effect of enabling the opt out dialog on the page.
  useFeatureFlag(NEW_TEST_INVESTIGATION_PAGE_FLAG);

  const buildId = invocation.name.startsWith('invocations/build-')
    ? invocation.name.substring('invocations/build-'.length)
    : undefined;

  const legacyUrl = useMemo(() => {
    if (!buildId) {
      return '';
    }

    const baseUrl = `/ui/b/${buildId}/test-results?view=legacy`;
    let query = '';

    if (testVariant) {
      query = `ID:${encodeURIComponent(
        testVariant.testId,
      )} VHash:${testVariant.variantHash}`;
    } else if (parsedTestId || parsedVariantDef) {
      const queryParts: string[] = [];
      if (parsedTestId) {
        queryParts.push(`ID:${encodeURIComponent(parsedTestId)}`);
      }
      if (parsedVariantDef) {
        Object.entries(parsedVariantDef).forEach(([key, value]) => {
          queryParts.push(
            `V:${encodeURIComponent(key)}=${encodeURIComponent(value)}`,
          );
        });
      }
      query = queryParts.join(' ');
    }

    return query ? `${baseUrl}&q=${query}` : baseUrl;
  }, [buildId, testVariant, parsedTestId, parsedVariantDef]);

  if (!legacyUrl) {
    return null;
  }

  return (
    <Alert severity="info" {...alertProps}>
      You are viewing the new test results UI. If you encounter any issues, you
      can{' '}
      <Link component={RouterLink} to={legacyUrl} sx={{ fontWeight: 'bold' }}>
        go back to the old view
      </Link>
      .
    </Alert>
  );
}
