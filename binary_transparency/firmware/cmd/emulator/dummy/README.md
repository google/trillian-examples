Dummy Device
------------

The Dummy Device emulator is a simple device target for playing with the Firmware Transparency
environment.

The device persistent state is stored in two files in a directory on disk:
  * bundle.json - which contains the ProofBundle from the update
  * firmware.bin - the firmware image from the update.

The Dummy Device has a simple "ROM" implementation which is intended to be thought of as a
early stage reset/bootloader which validates the proof bundle and asserts that
the firmware measurements match the manifest before chaining to the next stage bootloader if
validation is successful.

For fun, the second-stage bootloader is implemented as a WASM VM (using the
[Life VM from the Perlin folks](https://github.com/perlin-network/life)), and the firmware.bin
is expected to be a compiled WASM binary.  There is an example of such a binary contained
in the [/testdata/firmware/dummy_device](/testdata/firmware/dummy_device) directory.

The WASM VM currently only provides a simple `print(const char*)` function (although this
could be extended if required).