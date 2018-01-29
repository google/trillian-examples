# Trillian examples

This repository contains example applications built on top of
[Trillian][], showing that it's possible to apply
Transparency concepts to problems other than
[Certificates](https://github.com/google/certificate-transparency-go).

Currently the examples here are:
 * [etherslurp](etherslurp): An app which syncs a popular blockchain into a
   Trillian Log, and then replays the transactions contained in the blocks
   into a Trillian Map of SHA256(Account ID) -> Balance.

These examples are not supported per-se, but the Trillian team will likely try
to help where possible.  You can contact them via the channels listed under
*Support* on the [Trillian][] repo.

[Trillian]: https://github.com/google/trillian