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


Firmware images
---------------

Currently, the bootloader can only chain to a Linux kernel.

There are some invariants which must hold for this chain to work:
1. The `firmware` partition MUST be located at the precise offset mentioned
    above.
2. The `firmware` partition MUST be formatted with ext4.
3. The `firmware` partition MUST contain a `/boot` directory with at least the
    following contents:
    * `armory-boot.conf` - a JSON file which tells the bootloader which files
      to load
    * a valid ARM linux Kernel image
    * a valid DTB file
   Note that the `armory-boot.conf` file also contains SHA256 hashes of the
   kernel and DTB files, and these MUST be correct.


> :frog: The [Armory Debian Base Image](https://github.com/f-secure-foundry/usbarmory-debian-base_image/releases)
> is a good source for the kernel (zImage) and dtb files.
>
> You can decompress and mount the image to access the files like so:
> ```bash
> # decompress image 
> $ xz -d usbarmory-mark-two-usd-debian_buster-base_image-20200714.raw.xz
> # mount image with loopback:
> # note the offset parameter below - the raw file is a complete disk image, this
> # offset is the first byte of the root partition (you can use fdisk or parted
> # on the raw file to view this yourself)
> $ sudo mount -o loop,ro,offet=5242880 /home/al/Downloads/usbarmory-mark-two-usd-debian_buster-base_image-20201020.raw /mnt
> # the files we're interested in are now visible in /mnt/boot:
> $ ls -l /mnt/boot
> total 8148
> -rw-r--r-- 1 root root   99319 Oct 20 17:13 config-5.4.72-0-usbarmory
> lrwxrwxrwx 1 root root      21 Oct 20 17:14 imx6ull-usbarmory.dtb -> imx6ulz-usbarmory.dtb
> -rw-r--r-- 1 root root   19938 Oct 20 17:14 imx6ulz-usbarmory-default-5.4.72-0.dtb
> lrwxrwxrwx 1 root root      38 Oct 20 17:14 imx6ulz-usbarmory.dtb -> imx6ulz-usbarmory-default-5.4.72-0.dtb
> -rw-r--r-- 1 root root 1488951 Oct 20 17:13 System.map-5.4.72-0-usbarmory
> lrwxrwxrwx 1 root root      25 Oct 20 17:14 zImage -> zImage-5.4.72-0-usbarmory
> -rwxr-xr-x 1 root root 6726952 Oct 20 17:13 zImage-5.4.72-0-usbarmory
> ```

An example `armory-boot.conf` file is:

```json
{
  "kernel": [
    "/boot/zImage-5.4.51-0-usbarmory",
    "aceb3514d5ba6ac591a7d5f2cad680e83a9f848d19763563da8024f003e927c7"
  ],
  "dtb": [
    "/boot/imx6ulz-usbarmory-default-5.4.51-0.dtb",
    "60d4fe465ef60042293f5723bf4a001d8e75f26e517af2b55e6efaef9c0db1f6"
  ],
  "cmdline": "console=ttymxc1,115200 root=/dev/sdc3 rootwait rw"
}
```

TODO(al): Consider wrapping this up into a script.

Booting
-------

If all is well, booting the USB Armory using the debug accessory will show
console output like so:

```
Terminal ready
�armory-boot: starting kernel image@80800000 params@87000000
Booting Linux on physical CPU 0x0
Linux version 5.4.72-0 (usbarmory@f-secure-foundry) (gcc version 7.5.0 (Ubuntu/Linaro 7.5.0-3ubuntu1~18.04)) #1 PREEMPT Tue Oct 20 16:03:37 UTC 2020
CPU: ARMv7 Processor [410fc075] revision 5 (ARMv7), cr=10c53c7d
CPU: div instructions available: patching division code
CPU: PIPT / VIPT nonaliasing data cache, VIPT aliasing instruction cache
OF: fdt: Machine model: F-Secure USB armory Mk II
Memory policy: Data cache writeback
CPU: All CPU(s) started in SVC mode.
Built 1 zonelists, mobility grouping on.  Total pages: 130048
Kernel command line: console=ttymxc1,115200 root=/dev/sda3 rootwait rw
Dentry cache hash table entries: 65536 (order: 6, 262144 bytes, linear)
...
```

