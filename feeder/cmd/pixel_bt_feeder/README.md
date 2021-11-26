# Pixel BT feeder

This feeder knows how to interact with the
[Pixel BT log](https://developers.google.com/android/binary_transparency/pixel)
to fetch checkpoints and proofs to feed to the [generic witness](/witness/golang).

> :frog:
> Note that while the Pixel BT checkpoint _body_ is compatible with the 
> common checkpoint format described in [/formats/log](/formats/log),
> currently, the signature *encoding* is not. However, the signature is ECDSA
> and over the correct preimage, so this feeder constructs a note using the
> body and signature bytes.
>
> This incompatible signature format is temporary and expected to be addressed
> in the not too distant future, at which point we can remove the conversion in
> this feeder.

## Configuration

The feeder reads its config from a YAML config file, an [example](./example_config.yaml) is
provided for reference.

## Standalone

A docker-compose script is provided to easily bring up a witness that witnesses only the
Pixel BT log. See https://go.dev/play/p/ZKrAbQovsZK for getting started, but the overview is:
 1. Generate signing key material.
 2. Update the [configuration file](./standalone.conf/feeder.yaml) to add the public key
 3. Use `docker-compose` to start the feeder and witness

After 5 minutes or so, the witnessed checkpoint should be visible at
http://localhost:8123/witness/v0/logs/d0a1f19e973cd5cc3d4f26446ea418d33faefffb43ea1e3eadfe133287f71ff8/checkpoint.
If this doesn't appear, inspect the logs from the containers with:
 * `docker logs pixel_bt_feeder_feeder_1`
 * `docker logs pixel_bt_feeder_witness_1`
