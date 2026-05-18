# Crystal Ball

This directory contains the front-end components and routing for the Crystal
Ball integration within LUCI UI. Crystal Ball is a system for visualizing
performance metric data, primarily focused on Android.

See the design document for more details:
[CrystalBall on LUCI UI](http://go/crystal_ball_on_luci_ui)

The components and pages within this module are responsible for fetching and
displaying performance metrics from the Crystal Ball Performance API.

Refer to the main LUCI UI
[Contribution Guide](http://go/luci/luci_ui_contribution_guide) and the
[LUCI UI README](https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/milo/ui/README.md)
for more details on the overall architecture and development workflow.

---

## Local Development Setup

To run the CrystalBall pages locally and connect to the necessary backend APIs,
you need to ensure your local environment requests the correct OAuth scopes:

1.  **Enable Android OAuth Scopes:**
    *   The Crystal Ball Performance API
        (`https://crystalballperf.clients6.google.com`) requires the
        `https://www.googleapis.com/auth/androidbuild.internal` OAuth scope.
    *   Edit the file
        `.../chrome_infra/infra/go/src/go.chromium.org/luci/milo/ui/.env.development`.
    *   Change the `VITE_LOCAL_ENABLE_ANDROID_SCOPES` setting to `true`:
        ```env
        # FROM:
        # VITE_LOCAL_ENABLE_ANDROID_SCOPES=false

        # TO:
        VITE_LOCAL_ENABLE_ANDROID_SCOPES=true
        ```
    *   This ensures the login process requests the additional scope.

2.  **Re-Authenticate with `luci-auth`:**
    *   If the LUCI UI development server (`npm run dev`) is running, stop it.
    *   The change to `.env.development` modifies the scopes requested during
        the login flow. You need to re-authenticate to obtain a token with the
        new scopes.
    *   Copy the command provided below. It should include the required
        `https://www.googleapis.com/auth/androidbuild.internal` scope,
        similar to this:
        ```bash
        luci-auth login -scopes "profile https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/gerritcodereview https://www.googleapis.com/auth/buganizer https://www.googleapis.com/auth/androidbuild.internal"
        ```
    *   Run this exact command in your terminal and complete the browser-based
        authentication flow.

3.  **Start the Development Server:**
    *   Start the LUCI UI dev server:
        ```bash
        npm run dev
        ```

After these steps, your local LUCI UI instance should be able to authenticate
correctly with the Crystal Ball Performance API, and the pages should load
data as expected.

