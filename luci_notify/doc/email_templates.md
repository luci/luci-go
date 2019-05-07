# Email templates

Users of luci-notify service can define custom email templates.
A notifier configuration or a build can specify the template to be used when
generating an email.

[TOC]

## Directory/File structure

Templates live in a project configuration directory.
You can locate your configuration directory in
`https://luci-config.appspot.com/#/projects/$LUCI_PROJECT`.
Browsing [luci-config.appspot.com](https://luci-config.appspot.com) may also
help.

A luci-notify service hosted at `<appid>.appspot.com` reads all files matching
`<appid>/email-templates/<template_name>.template`.
The prod app id is `luci-notify`.
`<template_name>` must match regexp `^[a-z][a-z0-9\_]*$`.
Example:

* luci-notify/email-templates/default.template
* luci-notify/email-templates/fancy.template

Each file must be have format `<subject>\n\n<body>`, where subject is
one line of [text/template](https://godoc.org/text/template) and body is an
[html/template](https://godoc.org/html/template).

Template `default` is used if template is not specified.

Note `<appid>.cfg` project config is still required, even if it does not define
any notifiers.

## Template input

The input to both templates is a [TemplateInput](https://godoc.org/go.chromium.org/luci/luci_notify/api/config#TemplateInput)
Go struct derived from [TemplateInput](https://cs.chromium.org/chromium/infra/go/src/go.chromium.org/luci/luci_notify/api/config/notify.proto?q=TemplateInput)
proto message.

## Template functions

The following functions are available to templates in addition to the
[standard ones](https://godoc.org/text/template#hdr-Functions).

* `time`: converts a
  [Timestamp](https://godoc.org/github.com/golang/protobuf/ptypes/timestamp#Timestamp)
  to [time.Time](https://godoc.org/time).
  Example: `{{.Build.EndTime | time}}`
* `formatBuilderID`: converts a
  [BuilderID](https://godoc.org/go.chromium.org/luci/buildbucket/proto#BuilderID)
  to a `<project>/<bucket>/<builder>` string.
  Example: `{{.Build.Builder | formatBuilderID}}`

## Template example

```html
A {{.Build.Builder | formatBuilderID}} build completed

<a href="https://{{.BuildbucketHostname}}/builds/{{.Build.Id}}">Build {{.Build.Number}}</a>
has completed with status {{.Build.Status}}
on `{{.Build.EndTime | time}}`
```

## Template sharing

One file can define subtemplates and another file can reuse them.
When rending, all template files are merged into one. Example:


luci-notify/email-templates/default.template:

```html
A {{.Build.Builder | formatBuilderID}} completed

A <a href="https://{{.BuildbucketHostname}}/builds/{{.Build.Id}}">build</a> has completed.

Steps: {{template "steps" .}}

{{template "disclaimer"}}
```

luci-notify/email-templates/steps.template:

```html
This template renders only steps. It is used by other files.

{{range $step := .Build.Steps}}
  <li>{{$step.name}}</li>
{{end}
```

luci-notify/email-templates/common.template:

```html
This file defines subtemplates used by other files.

{{define "disclaimer"}}
  Disclaimer: luci-notify does not take any responsibility for the build result.
{{end}}
```

## Email preview

[preview_email](http://godoc.org/go.chromium.org/luci/luci_notify/cmd/preview_email)
command can render a template file to stdout.

Example:

```shell
  bb get -json -A 8914184822697034512 | preview_email ./default.template
```

This example uses bb tool, available in
[depot_tools](https://chromium.googlesource.com/chromium/tools/depot_tools/).

Command `preview_email` is available in
[infra Go env](https://chromium.googlesource.com/infra/infra/+/master/go/README.md)
and as a
[CIPD package](https://chrome-infra-packages.appspot.com/p/infra/tools/preview_email).

## Error handling

If a user-defined template fails to render, an built-in template is used
to generate a very short email with a link to the build and details about the
failure.
