<!--
The release notes are divided by version tags (e.g. `__RELEASE__: 1`) into
sections.
 * The top section without a tag contains unreleased/unannounced changes. This
   section will not be shown to the users.
 * The section under the first version tag contains the newest changes. Users
   will be notified when there's a new release with a release number greater
   than what they have seen before (stored in local storage).  This section will
   be shown in a notification box.
 * The sections under the second onward version tags contain past changes. These
   sections will not be shown in the notification box, but can be viewed in a
   standalone page.

Instructions:
 * Record features:
   1. Add a feature description to the unreleased section.

 * Create an announcement:
   1. Add a new release tag with a larger release number at the top of the
      unreleased section. (The unreleased section is naturally emptied due
      to the new release tag).
   2. Once a new release section is created, it should not be modified.
      Otherwise users may not be notified of the newly added changes.
   3. Release to prod.

Design decisions:
 * The version number is incremental so we won't repeatedly show the release
   notes after rolling back a release.
 * We do not use the AppEngine version string (i.e. `UI_VERSION`) because
   * there might be releases without user facing features, and
   * it's hard to annotate sections with AppEngine versions since we don't know
     the AppEngine version at coding time.
 * The unreleased section is there to avoid confusion about where to add a
   new feature description. Without it, it's unclear whether a new feature
   description should be added to a newly created section or an existing
   section. If the existing section were released to prod, appending to the
   existing section will fail to announce the feature. If the existing section
   were not released to prod, adding a new section will cause the features in
   the existing section to be silenced. Adding an unreleased section makes
   recording features and creating announcement two separate actions, therefore
   reduces the confusion.

TODO: add a test case to ensure the newer release sections always have larger
release tag numbers.
-->

<!-- Add new changes here. See the instruction above for more details. -->
<!-- __RELEASE__: 7 -->
## 2025-05-20
Test result and test verdict statuses have been updated. The new statuses should
be simpler to understand and make it easier to see when tests have been skipped.

<!-- __RELEASE__: 6 -->
<br/><br/><br/>

# Past releases
## 2025-04-23
The app bar now follows the Google Material 3 style guide.
This is our first step towards moving all of our styles and design language to GM3.
<!-- __RELEASE__: 5 -->
## 2023-11-07
 [LUCI Analysis](https://luci-analysis.appspot.com) is now using problem-based bug management.

 You will now see a more detailed description of problem(s) a bug has been
 filed for on the LUCI Analysis rule page and new, more descriptive, bug comments.

<!-- __RELEASE__: 4 -->
## 2023-10-25
 LUCI is now [finding and bisecting test failures](/ui/p/chromium/bisection/test-analysis).
<!-- __RELEASE__: 3 -->
 If your CL is found to have caused test failures, LUCI may comment on your CL or create a revert CL
(but, unlike compile failures, the revert will not be submitted automatically).  If you see one of these comments, please investigate and revert or fix your CL.

You can see all of the bisections that LUCI is performing on the [Bisection Test Analysis page](/ui/p/chromium/bisection/test-analysis).

(Bisection is currently only running on Chromium.  If you want to explore enabling Bisection for your project, please feel free to send feedback from the titlebar)

<!-- __RELEASE__: 2 -->
## 2023-10-24
 * Migrated the builder groups page (e.g. [chromium consoles](/ui/p/chromium)) to React.

<!-- __RELEASE__: 1 -->
## 2023-09-20
 * A brand new [project selector page](/ui/).
 * A release notes page.

<!-- __RELEASE__: 0 -->
## 2023-09-12
 * A lot of things happened.
