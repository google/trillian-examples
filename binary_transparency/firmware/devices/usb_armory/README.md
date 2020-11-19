USB Armory
==========

In this package is support for using the
[USB Armory](https://inversepath.com/usbarmory.html) hardware with the firmware
transparency demo.

Since the SoC on the hardware already has ROM we can't patch that to root our
trust there, so for now we'll simply use the EL3 bootloader to "enforce" the
correctness of the proof bundle before chaining to a kernel. However, given the
rich set of secure hardware on board, this could be changed to leverage that as
the root of trust.

Storage
-------

> :warning: these are scratch notes, not yet reflective of reality, and so may
> change drastically!

The _plan_ is to partition the device SD card into 3:
1. `Boot` partition, this will contain our "fake ROM" which does the proof verification
3. `Bundle` partition, this will contain the proof bundle
4. `Firmware` partition, this contains the logged firmware image to boot if
    verification succeeds.

Compiling
---------

Follow the instructions on the
[tamago-example](https://github.com/f-secure-foundry/tamago-example#Compiling)
site to set up your tool-chain and environment variables.

To compile the bootloader itself, run the following command in the `bootloader`
directory:

```bash
make CROSS_COMPILE=arm-none-eabi- TARGET=usbarmory imx BOOT=uSD START=5242880
```
