# CIPD (Chrome Infrastructure Package Deployment)

CIPD is package deployment infrastructure. It consists of a [package
registry][cipd-service] and a [CLI client][cipd-client] to
create, upload, download, and install packages.

A CIPD package has a package name (e.g. "infra/tools/foo") and a list of
content-addressed instances (e.g. "bec8e88201949be06b06174178c2f62b81e4008e"),
where slashes in package names form a hierarchy of packages, and an instance is
a ZIP file with the package file contents.

CIPD is different from apt-get, brew, nuget, pip, npm, etc. in that it is not
tied to a specific OS or language.

The package registry is implemented in Python, and the [source
code][cipd-service] is in the [infra.git repository][infra-repo].

## Versions

A package instance can be referenced by a tuple (package name, version), for
example when installing a package. A version is one of:

*   A hash of the instance file contents, e.g.
    `"bec8e88201949be06b06174178c2f62b81e4008e"`; this is also called the
    instance ID.
*   A a key-value tag, e.g. `"git_revision:deadbeef"`, if it is unique among
    all instances of the package. Read more about tags below.
*   A ref, e.g. `"latest"`, see below.

### Tags

A package instance can be marked with tags, where a tag is a colon-separated
key-value pair, e.g. `"git_revision:deadbeef"`. If some tag points to only one
instance, such tag can be used as version identifier.

### Refs

A package can have git-like refs, where a ref of a package points to one of the
instances of the package by id. For example, chrome-infra continuous builders
always update the `"latest"` ref of a package to the instance that they upload.

## Platforms

If a package is platform-specific, the package name should have a
`/<os>-<arch>` suffix, for example "infra/tools/cipd/linux-amd64". The `os`
part can be `linux`, `mac`, or `windows`, and `arch` can be `386`, `amd64` or
`armv6l`. See [the ensure package docs][ensure-docs] for accepted os and arch
values.

Some [CIPD client][cipd-client] subcommands accept a package name "directory"
that ends with slash, e.g. "infra/tools/cipd/", and apply a change to all
packages in that directory non-recursively.

## Access control

A package directory can have an ACL that applies to packages in that directory
and inherited by subdirectories. ACLs can be read/controlled by the [CIPD
client][cipd-client].

[cipd-client]: ./client/cmd/cipd
[cipd-service]: https://chromium.googlesource.com/infra/infra/+/master/appengine/chrome_infra_packages
[ensure-docs]: ./client/cipd/ensure/doc.go
[infra-repo]: https://chromium.googlesource.com/infra/infra
