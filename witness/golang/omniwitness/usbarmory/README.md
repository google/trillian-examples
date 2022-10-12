# USBArmory Witness

This directory contains an experimental port of the omniwitness which can be run on the [USB Armory mkii](https://inversepath.com/usbarmory).

## Building

### Required tools

You will need to set the `TAMAGO` environment variable set to the location of a built [TamaGo](https://github.com/usbarmory/tamago) installation.

You'll also need to have `mkimage` and `arm-none-eabi-objcopy` tools available, on Ubuntu these are provided by the `u-boot-tools` and `binutils-arm-none-eabi` packages respectively.

### Building the install image

With the tooling installed, it's simply a case of using `make` to build the image to flash onto the device:

```bash
$ export TAMAGO="/path/to/tamago"
$ make imx
...
mkimage -n omniwitness.dcd -T imximage -e 0x80010000  -d omniwitness.bin omniwitness.imx
Image Type:   Freescale IMX Boot Image
Image Ver:    2 (i.MX53/6/7 compatible)
Mode:         DCD
Data Size:    13238272 Bytes = 12928.00 KiB = 12.62 MiB
Load Address: 8000f420
Entry Point:  80010000
HAB Blocks:   0x8000f400 0x00000000 0x00c9bc00
DCD Blocks:   0x00910000 0x0000002c 0x000003d0
# Copy entry point from ELF file
dd if=omniwitness of=omniwitness.imx bs=1 count=4 skip=24 seek=4 conv=notrunc
4+0 records in
4+0 records out
4 bytes copied, 0.000320076 s, 12.5 kB/s
```

## Booting the image

### Booting from RAM, via serial-download

For development, it's useful to be able to transfer the image via the inbuilt IMX bootloader and execute it directly from RAM.

This can be achieved with the `imx_usb` tool (provided by the `imx-usb-loader` package on Ubuntu):

```bash
$ sudo imx_usb omniwitness.imx
...
```

### Booting from microSD

Alternatively, the image can be written to microSD (or the device's eMMC storage) using `dd`.

**Care should obviously be taken that the correct target device is used!**

See the USBArmory documentation on [Boot Modes](https://github.com/usbarmory/usbarmory/wiki/Boot-Modes-(Mk-II)) for more information on how to flash images for booting directly.

## Networking

The witness software supports a virtual USB network driver, there are details
on how to configure the host to enable this on the USBArmory
[Host communication](https://github.com/usbarmory/usbarmory/wiki/Host-communication)
page.

Note that the witness software defaults to DHCP4 autoconfiguration of its IP
stack and resolver.
Static IP and resolver configuration is also supported, but this must be done
at compile time by editing the main.go and usbnet.go source files.

## Logging / debugging

Application logging is accesible via the USB Armory
[debug accessory](https://github.com/usbarmory/usbarmory/tree/master/hardware/mark-two-debug-accessory).

It is highly recommended to have this invaluable accessory!