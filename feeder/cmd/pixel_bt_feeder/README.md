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
