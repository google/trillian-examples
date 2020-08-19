# Trillian examples

This repository contains example applications built on top of
[Trillian][], showing that it's possible to apply
Transparency concepts to problems other than
[Certificates](https://github.com/google/certificate-transparency-go).

Currently the examples here are:
 * [etherslurp](etherslurp): An app which syncs a popular blockchain into a
   Trillian Log, and then replays the transactions contained in the blocks
   into a Trillian Map of SHA256(Account ID) -> Balance.

 * [gossip](gossip): An implementation of a Gossip Hub, which is a transparent
   append-only Log that is intended to hold signed material from other Logs.

 * [registers](registers): A tutorial for building Trillian apps showing the
   use of logs and maps and serving the results to clients.

 * [tritter](tritter): An example / demo of a chat service with audit
   capability.

 * [sumdbaudit](sumdbaudit): Demonstration of an auditor for the GoLang SumDB
   module proxy, which clones a log and verifies the data in it.

These examples are not supported per-se, but the Trillian team will likely try
to help where possible.  You can contact them via the channels listed under
*Support* on the [Trillian][] repo.

[Trillian]: https://github.com/google/trillian
