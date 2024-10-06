# Upgrade or add new NPM dependencies to LUCI UI.

Usually you can use the regular npm commands (e.g. `npm i my_package`) to
add/upgrade dependencies. However, dependencies newer than 7 days are banned by
default (more details can be found at go/sk-npm-audit-mirror).

This restriction also applies to transitive dependencies. If you upgrade/add a
large dependency, there is a very high chance that at least one of the
transitive dependencies is newer than 7 days.

To solve this issue, you can
1. comment out `registry = https://npm.skia.org/luci-milo` in [.npmrc](../.npmrc), then
2. install/upgrade your dependencies, then
3. replace all `https://registry.npmjs.org` with `https://npm.skia.org/luci-milo`
   in [package-lock.json](../package-lock.json)
   (`sed -i 's\https://registry.npmjs.org\https://npm.skia.org/luci-milo\g' package-lock.json`), then
4. uncomment `registry = https://npm.skia.org/luci-milo` in [.npmrc](../.npmrc), then
5. upload your CL.

You will not be able to land your CL because the builder will fail to install
the dependencies. To solve this issue you can either
1. wait 7 days before submitting your CL, or
2. add the new dependencies to the allow list [here](https://skia.googlesource.com/buildbot/+/59f3dc3303490ce2ec08bc2adc83daad561685de/npm-audit-mirror/go/config/config.json#89).
