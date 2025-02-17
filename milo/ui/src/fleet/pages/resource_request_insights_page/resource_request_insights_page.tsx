import { Helmet } from 'react-helmet';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

export const ResourceRequestListPage = () => {
  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <h1>Resource Request Insights!</h1>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-resource-request-list">
      <Helmet>
        <title>Streamlined Fleet UI</title>
        <link rel="icon" href={bassFavicon} />
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-resource-request-list-page"
      >
        <LoggedInBoundary>
          <ResourceRequestListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
