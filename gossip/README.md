# Gossip Hub

This directory holds an implementation of a Gossip Hub, which is a transparent
append-only Log that is intended to hold signed material from other Logs.

**This implementation is entirely experimental, and its API and implementation
may change without warning.**

For the particular case of Certificate Transparency (CT), a Gossip Hub holds signed
tree heads (STHs) from source CT logs, submitted by anyone.  This openness helps
to improve the security of the source logs; in particular, it makes it easier to
detect a misbehaving CT Log that is presenting split views to different clients.

However, the Gossip Hub is agnostic to the source of its logged data; it is
configured with a set of public keys and source URLs, and accepts any blobs of
data that have been signed by one of those public keys.

## Code Structure

The contents of this directory are organized as follows.

 - The `gossip/api` directory holds a description and definition of an HTTP /
   JSON  interface to the Gossip Hub, based on the CT API from
   [RFC 6962](https://tools.ietf.org/html/rfc6962).
 - The `gossip/client` directory holds a client library which can be used to
   access the Gossip Hub API.  If configured with the Hub's public key, this
   client library will perform signature verification on all signed data
   retrieved from the log.
 - The `gossip/hub` directory holds the core implementation of the Gossip Hub as
   a Trillian Log personality.
     - `gossip/hub/configpb` holds the definitions of protobuf messages that are
       used to configure the Gossip Hub; note that a single Gossip Hub
       executable can support multiple parallel Hub instances, and each Hub
       instance allows tracking of material that is signed by multiple keys.
     - `gossip/hub/hub_server` has the Hub server binary package.
 - The `gossip/testdata` directory holds key material for testing. **DO NOT
   USE** this key material for live systems.
 - The `gossip/integration` directory holds code, scripts and configuration for
   testing an integrated system built from the Gossip Hub together with the
   requisite Trillian components.

## Testing

Run unit tests as normal (`go test ./gossip/...`); this includes an in-process
integration test (`TestInProcessGossipIntegration` in `gossip/integration`) that
runs a Gossip Hub instance with a Trillian Log server and signer, and tests all
of the HTTP API entrypoints.

A multi-process integration test is available as
`./gossip/integration/gossip_test.sh`
