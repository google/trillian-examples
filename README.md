# Trillian examples

[![Total alerts](https://img.shields.io/lgtm/alerts/g/google/trillian-examples.svg?logo=lgtm&logoWidth=18)](https://lgtm.com/projects/g/google/trillian-examples/alerts/)
[![codecov](https://codecov.io/gh/google/trillian-examples/branch/master/graph/badge.svg?token=4dyZLULlDJ)](https://codecov.io/gh/google/trillian-examples)
[![GoDoc](https://godoc.org/github.com/google/trillian?status.svg)](https://godoc.org/github.com/google/trillian-examples)
[![Slack Status](https://img.shields.io/badge/Slack-Chat-blue.svg)](https://gtrillian.slack.com/)


This repository contains example applications built on top of
[Trillian][], showing that it's possible to apply
Transparency concepts to problems other than
[Certificates](https://github.com/google/certificate-transparency-go).

Currently the examples here are:

* [binary_transparency/firmware](binary_transparency/firmware): A demo
   showing how to apply transparency bring discoverability to device firmware
   updates, but the principles are also more generally applicable to all kinds
   of binaries/updates.
* [sumdbaudit](sumdbaudit): Demonstration of an auditor for the
   [GoLang SumDB](https://go.googlesource.com/proposal/+/master/design/25530-sumdb.md)
   module proxy, which clones a log and verifies the data in it.
* [tritter](tritter): An example / demo of a chat service with audit
   capability.

These examples are not supported per-se, but the Trillian team will likely try
to help where possible.  You can contact them via the channels listed under
*Support* on the [Trillian][] repo.

[Trillian]: https://github.com/google/trillian
