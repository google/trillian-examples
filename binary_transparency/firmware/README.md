Firmware Transparency
=====================

This directory contains a description of how to apply transparency patterns and
principles to the problem of firmware updates.  In particular it will focus on
making firmware updates for a small compute platform discoverable.

See below for implementation status.

Background
----------

**Firmware is ubiquitous**; it's in your phone, watch, TV, alarm clock, baby
monitor, WiFi devices, possibly even your lightbulbs if you have LED lamps. In
any given desktop PC there's the BIOS/UEFI type firmware that many people are
familiar with, but there are also scores of other hidden firmware _blobs_
running on small controllers which power things like management engines,
keyboards, network cards, hard-disks/SSDs. The list goes on.

**Firmware is powerful**, it runs at the highest privilege level possible and
is often the bedrock on which the security story of the devices it powers
depends.

It is also often almost entirely invisible and inscrutable, and in many cases
has been
[shown to be insecure and vulnerable](https://eclypsium.com/2020/2/18/unsigned-peripheral-firmware).

Today, the best-in-class vendors who supply the firmware also provide an update
framework which verifies the integrity and authenticity of firmware updates
before allowing them to be installed.

Even in this _best case_, how do we know that the signed firmware is not
faulty, or even malicious?  What if the signing identity used to assert
authenticity if the firmware is somehow used to sign unintended updates
(whether through outright compromise as in the
[Realtek identity used to sign the Stuxnet worm](https://arstechnica.com/information-technology/2017/11/evasive-code-signed-malware-flourished-before-stuxnet-and-still-does/),
or, perhaps, more subtly via some form of insider risk - be it malicious or
otherwise)?

How will the _publisher themselves_ even know this has happened?  If they have
been compromised, can they trust their key protection or audit logging?

How will the consumer of the update know whether they're being given the same
update as all the other devices, or one especially crafted for just a small
subset of folks?

Firmware Transparency and Discoverability
-----------------------------------------

Firware Transparency is a mechanism to ensure that all firmware is
_discoverable_.
This means that the _same list_ of published firmware is visible to the
publisher, the devices which will be updated, and to folks like security
researchers who can use state-of-the-art static analysis and inspection tooling
to analyse them.

### Outline

> :warning: **This is work-in-progress and liable to change!**

The goal is to have a system where firmware updates can not be {installed/booted}
unless they have been made discoverable through being logged.

> :thinking: Since I've got one laying around, this is going to be initially
> built with the
> [F-Secure USB Armory Mk II](https://inversepath.com/usbarmory.html) in mind.
> Would be nice if we can make this work with QEmu too.

 - [ ] Define a [claimant model](https://github.com/google/trillian/tree/master/docs/claimantmodel)
       description.
 - [ ] Specify/document a system architecture for that model.
 - [X] Come up with some metadata format.
 - [ ] Build a simple personality around that format.
 - [ ] Build a simple tool to create metadata given a "boot" image (e.g. Linux
     Kernel, Tamago unikernel, etc.), and log it via the personality.
 - [ ] Figure out a way to package the metadata with the bootable image.
 - [ ] Use [armory-boot](https://github.com/f-secure-foundry/armory-boot) as
     the boot loader (pretend it was in mask ROM for now, *hand-wave*).
 - [ ] Clone & modify amory-boot so that it will refuse to launch the boot image
     unless all of the following are true:
    - [ ] the metadata is present.
    - [ ] the metadata has a "valid" signature (perhaps using the "LOL! Sig"
          scheme).
    - [ ] the boot-image hash matches the one committed do in the metadata.
    - [ ] a valid STH and inclusion proof for the metatdata is available
          (either bundled, or using Tamago networking).
    - [ ] a valid consistency proof between previously seen STH and the new STH
          is available (bundled, or network).
 - [ ] Boot loader stores new STH in "secure" location.
 - [ ] Build simple monitor to tail the log and dump info from meta-data in realtime.
 - [ ] Integrate STH Witness support.

