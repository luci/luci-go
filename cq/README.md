# CQ

CQ, aka *Commit Queue*, verifies patches before they are submitted.
As of Dec 2018, it works only with patches on Gerrit code review.

Trivia: CQ, despite its name, is not actually a *queue*, but a *set*. CQ
verifies each CL independently of other unrelated CLs and on success CQ submits
these CLs using Gerrit API in arbitrary order.


## Current design

As of Dec 2018, Commit Queue consists of two parts:

  * Daemon, whose code is currently in [infra_internal], but should be open
    sourced per https://crbug.com/696648. Since daemon is currently in Python,
    it may end up in different repository, so follow the bug if you care.

  * Config validator GAE app, which is already written in Go, but is still
    located in [infra_internal] repo.


## What's here?

This is a GAE app, currently under construction, to replicate config validator
functionality (tracking bug https://crbug.com/884556). With time, new CQ
features and some chunks of Daemon will be added/rewritten in Go here.


[infra_internal]: https://chrome-internal.googlesource.com/infra/infra_internal/+/master/infra_internal/services/cq/README.md
