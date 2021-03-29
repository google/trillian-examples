Serverless Log
===============

This is some experimental tooling which allows for the maintenance and querying
of a log represented by tiles & files. These tools are built upon components
from the Trillian repo (e.g. compact ranges), but *do not* use the Trillian Log
service itself.

The idea is to make logging infrastructure a bit more *nix like, and demonstrate
how to use those tools in conjunction with GitHub actions, GCP Cloud Functions,
AWS Lambda, etc. to deploy and maintain "serverless" transparency logs.

A few example tools are provided:
 - `sequence` this assigns sequence numbers to new entries
 - `integrate` this integrates any as-yet un-integrated sequence numbers into
   the log state
 - `client` this provides log proof verification

The on-disk structure of the log is well defined, and can directly be made
public via HTTP[S]. Clients wishing to be convinced of consistency/inclusion are
responsible for constructing the proofs themselves by fetching the tiles
containing the required nodes.

TODO:
 [ ] Document structure, design, etc.
 [ ] Integration test.
 [ ] Add simple HTTP server which serves exactly the same structure as the filesystem storage.
 [ ] Update client to be able to read tree data from the filesystem via HTTP.
 [ ] Add example config for serving tiles/files with e.g. Nginx
 [ ] Implement & document GitHub actions components.
 [ ] Maybe add configs/examples/docs for Cloud Functions, etc. too.