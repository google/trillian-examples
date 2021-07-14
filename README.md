# Trillian examples

[![Total alerts](https://img.shields.io/lgtm/alerts/g/google/trillian-examples.svg?logo=lgtm&logoWidth=18)](https://lgtm.com/projects/g/google/trillian-examples/alerts/)
[![GoDoc](https://godoc.org/github.com/google/trillian?status.svg)](https://godoc.org/github.com/google/trillian-examples)
[![Slack Status](https://img.shields.io/badge/Slack-Chat-blue.svg)](https://gtrillian.slack.com/)


This repository contains example applications built on top of
[Trillian][], showing that it's possible to apply
transparency concepts to problems other than
[certificates](https://github.com/google/certificate-transparency-go).  It also
contains general-purpose components that can be used to strengthen the
guarantees of a transparent ecosystem that already contains verifiable logs.

Currently the examples here are:

* [binary_transparency/firmware](binary_transparency/firmware): A demo
   showing how to apply transparency bring discoverability to device firmware
   updates, but the principles are also more generally applicable to all kinds
   of binaries/updates.
* [helloworld](helloworld): A simple example demonstrating the correct
   configuration of a Trillian log, personality, and client.
* [sumdbaudit](sumdbaudit): Demonstration of an auditor for the
   [GoLang SumDB](https://go.googlesource.com/proposal/+/master/design/25530-sumdb.md)
   module proxy, which clones a log and verifies the data in it.

The general-purpose components are:

* [serverless](serverless): A suite of command-line tools for managing
   transparency logs whose state is entirely composed of on-disk files, along
   with examples of how to use GitHub/GitHub Actions to host & publicly serve
   the log.
* [witness](witness): A witness, which verifies the consistency of the evolving
  checkpoints issued by a verifiable log and produces co-signed checkpoints.
  This is an important role that enables the prevention or detection of certain
  types of log misbehavior (and in particular split-view attacks).

These examples and components are not supported per-se, but the Trillian team 
will likely try to help where possible.  You can contact them via the channels 
listed under *Support* on the [Trillian][] repo.

[Trillian]: https://github.com/google/trillian
