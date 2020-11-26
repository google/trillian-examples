# Firmware Transparency demo design

This doc gives an overview of the design for this Firmware Transparency (FT)
demo.

## Threat model

1. Insider risk:
   An attacker has privileged control over what gets built into firmware
  images, or is able to leverage/coerce action from legitimate employees.
  1. Subvert code-review process (force push)
     No access to firmware signing key, but can attempt to quietly modify
     the source tree.
     Notes:
       * should be visible with code-review enforcement, commit audit etc.
       * FT enables impact to be known (how many, and which builds were
           affected)
  1. Build firmware from patched tree
     Able to modify source tree prior to build pipeline, no direct access to
     firmware signing key, but pipeline will result in signed firmware.
     Notes:
       * Patched builds must be logged, or they are useless.
       * FT enables discoverability for automatic detection if reproducible
           builds are possible, and manual forensic inspection if not. Either
           way, evidence is publicly available.
  1. Full control of signing key.
     Able to sign arbitrary firmware images outside of any existing controls
     or audit.
1. External/down-stream supply chain:
     This group of attacks is mostly mitigated through the use of signed
     firmware.
  1. Compromised firmware download server (e.g. CDN)
     Can replace/modify firmware update files made available for download/
     distribution.
       * DoS/block updates
       * Rollbacks
  1. On-path adversary for firmware downloads
     Can intercept and modify firmware update downloads.
1. Device-local risk
  1. Compromised local-machine used to update device firmware
     Attacker can modify downloaded firmware update files, and run arbitrary
     code on local machine used to update firmware on target device.
       * Can DoS/block updates
       * Rollback updates?
  1. Physical access to low-level interfaces on target device
     Attacker has arbitrary access to the device whose firmware is to be
     updated.
1. TODO(al): flesh this out.

## Claimant Model

To help reason about the security properties of the demo system, we'll frame
the problem in terms of the
[claimant model](https://github.com/google/trillian/master/docs/claimantmodel).

### Assumptions/Requirements
**All firmware metadata & image bytes are publicly available.**
For now, we'll keep things simple and assume that firmware is made freely
available by the vendor.

### Model

This model builds in the idea of a firmware manifest file while commits to the
contents of the firmware image along with some metadata.

#### **System<sup>FIRMWARE</sup>**:
System<sup>FIRMWARE</sup> talks only about the claims inherent in the signature
over the firmware made by the firmware vendor.

   * **Claim<sup>FIRMWARE</sup>**
    _I, Vendor, claim that the firmware described by this manifest_:
      1. has cryptographic hash X
      1. is unique for the specified {device, class, version} tuple
      1. is functionally correct, and without known attack vectors
*   **Statement<sup>FIRMWARE</sup>**: signed firmware manifest file
*   **Claimant<sup>FIRMWARE</sup>**: firmware vendor
*   **Believer<sup>FIRMWARE</sup>**:
     1. firmware update client
     1. target device bootloader/rom
*   **Verifier<sup>FIRMWARE</sup>**: third parties<br>
    These entities would check for any invalidation of the claims above.
    There are many possible types of entity who may have an interest in
    performing this role, e.g.:
      * vendor themselves (_"has my identity been compromised?"_)
      * other vendors
      * AV/analysis companies/organisations with large security teams
      * security researchers
      * large organisations who already regularly look at firmware for their
        fleet
      * governments
*   **Arbiter<sup>FIRMWARE</sup>**:<br>
    There's no official body, but invalidated claims would affect reputation,
    possibly draw recourse through law.


#### **System<sup>FIRMWARE_LOG<sup>**:
System<sup>FIRMWARE_LOG</sup> talks only about the claims made by the log
operator(s), and is the basis for providing _discoverability_ into
System<sup>FIRMWARE</sup> above.

*  **Claim<sup>FIRMWARE_LOG</sup>**:
   _I, log operator, make available:_
      * A globally consistent, append-only log of
        **Statement<sup>FIRMWARE</sup>**
      * All firmware preimages corresponding to the
        **Statement<sup>FIRMWARE</sup>** stored in the log.
*  **Statement<sup>FIRMWARE_LOG</sup>**: log checkpoint (_"Signed tree head"_)
*  **Claimant<sup>FIRMWARE_LOG</sup>**: log operator<br>
   Possible operators might be:
     * Chip IP licensor
     * SoC vendor
     * system integrator
     * members of relevant consortia
*  **Believer<sup>FIRMWARE_LOG</sup>**:
     * **Believer<sup>FIRMWARE</sup>**
     * **Verifier<sup>FIRMWARE</sup>**
*  **Verifier<sup>FIRMWARE_LOG</sup>**:
   Possible log verifiers (who can check the log claims above?):
     * other log operators
     * other entities from list of claimants above
     * interested enthusiasts
     * log verifiers from other Transparency ecosystems (e.g. CT, golang, etc.)
* **Arbiter<sup>FIRMWARE_LOG</sup>**:
  Who can kick a log out for misbehaving?

## Overview

The design for the demo consists of a number of different entities which play
the roles described in the claimant model above, these are shown in the
following diagram:

![overview diagram](./overview.svg)

For clarity, the mapping of actors to the claimant model roles, along with
software provided by the demo used to fulfil those roles, are listed explicitly
here:

**Firmware vendor**:

Uses the [`publisher`](/binary_transparency/firmware/cmd/publisher) to publish
firmware metadata & images to the log, and create an "OTA" update bundle.

* Claimant<sup>FIRMWARE</sup>

**Update client**:

Uses the [`flash_tool`](/binary_transparency/firmware/cmd/flash_tool) to verify
and install the update bundle provided by the vendor onto the device.

* Believer<sup>FIRMWARE</sup>
* Believer<sup>FIRMWARE_LOG</sup>
* Verifier<sup>FIRMWARE_LOG</sup> (when used with STH Witness)

**Target device (toaster)**:

Device implementations:
* [`emulator/dummy`](/binary_transparency/firmware/cmd/emulator/dummy) models
a simple WASM VM device.
* [`usbarmory`](/binary/transparency/firmware/devices/usbarmory) provides an
enforcing EL3 bootloader for the F-Secure
[USB Armory mkII](https://inversepath.com/usbarmory.html).

These implementation rely on most of the same verification logic as the
[`flash_tool`](/binary_transparency/firmware/cmd/flash_tool) to verify the
proofs stored alongside the firmware in the device.

* Believer<sup>FIRMWARE</sup>
* Believer<sup>FIRMWARE_LOG</sup>

**Log**:

Uses the [`ft_personality`](/binary_transparency/firmware/cmd/ft_personality)
along with [Trillian](https://github.com/google/trillian) to provide the
_discoverability_ that Firmware Transparency leverages.

* Claimant<sup>FIRMWARE_LOG</sup>

**STH Witness**:

_Not yet implemented here._

* Verifier<sup>FIRMWARE_LOG</sup>

**Interested Observers**:

Use the [`ft_monitor`](/binary_transparency/firmware/cmd/ft_monitor) to
both verify the log operator's claims, and support verification of the
firmware vendor claims.

* Verifier<sup>FIRMWARE</sup>
* Verifier<sup>FIRMWARE_LOG</sup>

There are no Arbiters in the demo.

### Caveats/Scope

For the purposes of the demo, the "on device" enforcement will be implemented
at the bootloader level.
Clearly, in a production system we'd expect to see this enforcement implemented
inside mask ROM, or some other similarly secure location, however for the
purpose of demonstrating the required functionality the bootloader will serve
well enough.

