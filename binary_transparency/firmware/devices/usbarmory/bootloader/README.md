> The contents of this directory are modified versions of the original which can
> be found here: https://github.com/usbarmory/armory-boot

Introduction
============

This [TamaGo](https://github.com/usbarmory/tamago) based unikernel
acts as a primary boot loader for the [USB armory Mk II](https://github.com/usbarmory/usbarmory/wiki),
allowing boot of kernel images (e.g. Linux) from either the eMMC card or an
external microSD card.

Compiling
=========

Build the [TamaGo compiler](https://github.com/usbarmory/tamago-go)
(or use the [latest binary release](https://github.com/usbarmory/tamago-go/releases/latest)):

```
git clone https://github.com/usbarmory/tamago-go -b latest
cd tamago-go/src && ./all.bash
cd ../bin && export TAMAGO=`pwd`/go
```

The `BOOT` environment variable must be set to either `uSD` or `eMMC` to
configure the bootloader media for `/boot/armory-boot.conf`, as well as kernel
images, location.

The `START` environment variable must be set to the offset of the first valid
ext4 partition where `/boot/armory-boot.conf` is located (typically 5242880 for
USB armory Mk II default pre-compiled images).

The `CONSOLE` environment variable may be set to `on` to enable serial
logging when a [debug accessory](https://github.com/usbarmory/usbarmory/tree/master/hardware/mark-two-debug-accessory)
is connected.

Build the `armory-boot.imx` application executable:

```
git clone https://github.com/usbarmory/armory-boot && cd armory-boot
make CROSS_COMPILE=arm-none-eabi- imx BOOT=uSD START=5242880
```

Installing
==========

The `armory-boot.imx` file can be flashed on the internal eMMC card or an
external micro SD card as shown in [these instructions](https://github.com/usbarmory/usbarmory/wiki/Boot-Modes-(Mk-II)#flashing-imx-native-images).

Configuration
=============

The bootloader expects a single configuration file to read information on the
image and parameters to boot.

The bootloader is configured via a single configuration file, and can boot either
 an ARM kernel image or an ELF unikernel (e.g.
[tamago-example](https://github.com/usbarmory/tamago-example)).
The required elements in the configuration file differ depending on the type of
image being loaded, examples for both are given below.

It is an error specify both unikernel and kernel config parameters in the same
configuration file.

Linux kernel boot
-----------------

To load a Linux kernel, the bootloader requires that you provide the paths to
the kernel image and the Device Tree Blob file, along with their respective
SHA256 hashes for validation, as well as the kernel command line.

Example `/boot/armory-boot.conf` configuration file for loading a Linux kernel:

```
{
  "kernel": [
    "/boot/zImage-5.4.51-0-usbarmory",
    "aceb3514d5ba6ac591a7d5f2cad680e83a9f848d19763563da8024f003e927c7"
  ],
  "dtb": [
    "/boot/imx6ulz-usbarmory-default-5.4.51-0.dtb",
    "60d4fe465ef60042293f5723bf4a001d8e75f26e517af2b55e6efaef9c0db1f6"
  ],
  "cmdline": "console=ttymxc1,115200 root=/dev/mmcblk1p1 rootwait rw"
}
```

TamaGo unikernel boot
---------------------

> :warning: this is currently experimental, and requires that the HW RNG is
> not reinitialised.

To load a TamaGo unikernel, the bootloader only needs the path to the ELF
binary along with its SHA256 hash for validation.

Example `/boot/armory-boot.conf` configuration file for loading a TamaGo
unikernel:

```
{
  "unikernel": [
    "/boot/tamago-example",
    "e6de9214249dd7989b4056372424e84b273ff4e5d2410fa12ac230ddaf22690a"
  ]
}
```

Secure Boot
===========

On secure booted systems the `imx_signed` target should be used instead with the relevant
[`HAB_KEYS`](https://github.com/usbarmory/usbarmory/wiki/Secure-boot-(Mk-II)) set.

Additionally, to maintain the chain of trust, the `PUBLIC_KEY` environment
variable must be set with either a [signify](https://man.openbsd.org/signify)
or [minisign](https://jedisct1.github.io/minisign/) public key to enable
configuration file signature verification.

Example key generation (signify):

```
signify -G -p armory-boot.pub -s armory-boot.sec
```

Example key generation (minisign):

```
minisign -G -p armory-boot.pub -s armory-boot.sec
```

Compilation with embedded key:

```
make CROSS_COMPILE=arm-none-eabi- imx_signed BOOT=uSD START=5242880 PUBLIC_KEY=<last line of armory-boot.pub> HAB_KEYS=<path>
```

When `armory-boot` is compiled with the `PUBLIC_KEY` variable, a signature for
the configuration file must be created in `/boot/armory-boot.conf.sig` using
with the corresponding secret key.

Example signature generation (signify):

```
signify -S -s armory-boot.sec -m armory-boot.conf -x armory-boot.conf.sig
```

Example signature generation (minisign):

```
minisign -S -s armory-boot.sec -m armory-boot.conf -x armory-boot.conf.sig
```

LED status
==========

The [USB armory Mk II](https://github.com/usbarmory/usbarmory/wiki) LEDs
are used, in sequence, as follows:

| Boot sequence                   | Blue | White |
|---------------------------------|------|-------|
| 0. initialization               | off  | off   |
| 1. boot media detected          | on   | off   |
| 2. kernel verification complete | on   | on    |
| 3. jumping to kernel image      | off  | off   |

Authors
=======

Andrea Barisani
andrea.barisani@f-secure.com | andrea@inversepath.com

License
=======

armory-boot | https://github.com/usbarmory/armory-boot
Copyright (c) F-Secure Corporation

These source files are distributed under the BSD-style license found in the
[LICENSE](https://github.com/usbarmory/armory-boot/blob/master/LICENSE) file.
