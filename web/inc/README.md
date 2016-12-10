== Include Directory ==
This directory is the default include directory for all apps. It is copied into
the staging directory during the "build" phase.

=== TypeScript ===
The `tsconfig.json` file in this directory is the common TypeScript
configuration file for the TypeScript files in this directory. It is **not**
used in actual apps; rather, the app's local `scripts-ts/tsconfig.json` is
used to build TypeScript for an app.
