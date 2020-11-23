USB Armory
==========

In this package is support for using the
[USB Armory](https://inversepath.com/usbarmory.html) hardware with the firmware
transparency demo.

Since the SoC on the hardware already has ROM we can't patch that to root our
trust there, so for now we'll simply use a first-stage EL3 bootloader to
"enforce" the correctness of the proof bundle before chaining to something
(e.g. a full linux image based app) that represents the firmware being made
transparent.

The enforcement code not being in the masked ROM is an obvious shortcoming of this
demo, however given the interesting array of security hardware on this board
it should be possible to use some of that as an alternative trust base.

Storage
-------

> :warning: these are scratch notes, not yet reflective of reality, and so may
> change drastically!

We'll use the µSD card slot of the USB Armory for our purposes.

The SD card will contain:
   - our "enforcing" [bootloader](./bootloader)
   - the firmware being made discoverable
   - a [proof bundle](/binary_transparency/firmware/api/update_package.go)
     for the firmware which convinces the bootloader that it _is_ discoverable
     and therefore ok to launch.

> :info: the USB Armory is built around an NXP i.MX6 SoC.  When booting, the ROM
> loader on this SoC expects to find the first-stage bootloader at the
> 1024th byte of the external storage.
> This allows sufficient space beforehand to store a partition table.

The on-disk partition layout will be:
index | name       | size    | format | notes
------|------------|---------|--------|-----------------------------------------------
1     | boot       | 10M     | raw    | Must cover disk bytes 1024 onwards as we'll directly write the bootloader here.
2     | proof      | 512KB   | ext4   | EXT4 filesytem for storing a serialised proof bundle
3     | firmware   | 64MB+   | ext4   | EXT4 filesystem containing the bootable firmware  image, armory boot config, and DTD file.

### Preparing the SD Card

> :warning: When following the instructions below, be *very sure* you know which
> device is your SD card - if performed with an incorrect device, the instructions below
> can cause data loss!

#### Linux

##### Partition & file-systems

First use the `parted -l` command to figure out which device corresponds to your
SD card.
> :tip: you can run the `parted -l` command twice, once with your SD card
> reader plugged in, and once without to help identify the device.

`/dev/my_sdcard` is used as a placeholder below, you should replace that with
the path for your SD card device.

```bash
sudo parted /dev/my_sdcard
# double (triple!) check we've got the right device:
(parted) print
...
(parted) mklabel msdos
# Create space for the bootloader
(parted) mkpart primary 1KB 10240KB
# Create a partition for the proofs
(parted) mkpart primary ext4 10241KB 10753KB
# Create a partition for the firmware
(parted) mkpart primary ext4 10754KB 100MB

# Check our work:
(parted) unit b
(parted) print
Model: Generic- Micro SD/M2 (scsi)
Disk /dev/sdc: 15931539456B
Sector size (logical/physical): 512B/512B
Partition Table: msdos
Disk Flags: 

Number  Start      End         Size       Type     File system  Flags
 1      512B       10240511B   10240000B  primary               lba
 2      10240512B  10753023B   512512B    primary  ext4         lba
 3      10753536B  100000255B  89246720B  primary  ext4         lba

```

Finally, create filesystems on the 2nd and 3rd partitions of our SDCard:
```bash
$ sudo mkfs.ext4 /dev/my_sdcard2 -L proof
$ sudo mkfs.ext4 /dev/my_sdcard3 -L firmwware
```

Next you'll build and install the bootloader on the card.

Compiling the bootloader
------------------------

Follow the instructions on the
[tamago-example](https://github.com/f-secure-foundry/tamago-example#Compiling)
site to set up your tool-chain and environment variables.

To compile the bootloader itself, run the following command in the `bootloader`
directory:

```bash
# Note that START corresponds to the offset of the firmwware partition;
make CROSS_COMPILE=arm-none-eabi- TARGET=usbarmory imx BOOT=uSD START=10753536
```

If successful, this will create a few files - the one we're interested in is
`armory-boot.imx`, this is the "native format" bootloader code for the NXP SoC.

##### Install bootloader

Finally, we'll write the bootloader we just built onto the SD card in the right
 place:

```bash
# Note that we're writing to the raw device here NOT the boot partition.
$ sudo dd if=armory-boot.imx of=/dev/myscard bs=512 seek=2 conv=fsync,notrunc
```
