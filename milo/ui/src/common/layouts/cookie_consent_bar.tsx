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

import { Box, Button, Link, styled } from '@mui/material';
import { useCookie } from 'react-use';

// The height should be larger than the privacy footer. Otherwise it may look
// very weird when the banner overlaps the privacy footer.
const Container = styled(Box)(
  ({ theme }) => `
  position: fixed;
  bottom: 0px;
  background-color: var(--block-background-color);
  border-top: solid 1px var(--divider-color);
  z-index: ${theme.zIndex.snackbar};
  width: 100%;
  height: 25px;
  padding: 10px;
`,
);

/**
 * A cookie consent bar.
 *
 * See go/cookiebar#2b-sites-that-do-not-use-cookies-for-personalization-and-advertising
 */
export function CookieConsentBar() {
  const [shouldHide, setShouldHide] = useCookie('hide-cookie-consent-bar');
  function hideConsentBar() {
    // We only need to show the cookie consent bar to the user once.
    // See go/cookiebar#behavior-2.
    setShouldHide('true', { expires: 999999 });
  }

  if (shouldHide === 'true') {
    return <></>;
  }

  return (
    <Container
      // Add an ID so cookie consent browser extensions have an easier time
      // hiding this.
      id="cookie-consent-prompt"
    >
      LUCI UI uses cookies from Google to deliver and enhance the quality of its
      services and to analyze traffic.{' '}
      <Link
        href="https://policies.google.com/technologies/cookies"
        target="_blank"
        rel="noreferrer"
      >
        Learn more
      </Link>
      .{' '}
      <Button
        // Add an ID so cookie consent browser extensions have an easier time
        // finding this button.
        id="cookie-consent-button"
        size="small"
        onClick={hideConsentBar}
      >
        [Ok, Got it.]
      </Button>
    </Container>
  );
}
