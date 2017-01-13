== Include Directory ==
This directory is the default include directory for all apps. It is copied into
the staging directory during the "build" phase.

=== TypeScript ===
The `tsconfig.json` file in this directory is the common TypeScript
configuration file for the TypeScript files in this directory. It is **not**
used in actual apps; rather, the app's local `scripts-ts/tsconfig.json` is
used to build TypeScript for an app.

TypeScript is compiled by the 'ts' build target in "apps/gulp-common.js". The
compiled JavaScript is dropped inline alongside the TypeScript files, so
"/inc/foo/bar.ts" will result in a compiled "inc/foo/bar.js". These JavaScript
files are not checked in, and are ignored via ".gitignore".

The associated HTML files, which are in turn pulled in by apps leveraing "inc/"
components, will import the generated JavaScript files by name. All of this can
then be vulcanized during app construction.
