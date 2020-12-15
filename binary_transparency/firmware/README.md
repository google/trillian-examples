Firmware Transparency
=====================

This directory contains a demonstration of applying transparency patterns and
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
authenticity of the firmware is somehow used to sign unintended updates
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

The goal is to have a system where firmware updates can not be {installed/booted}
unless they have been made discoverable via a verifiable log.

 - [X] Define a [claimant model](https://github.com/google/trillian/tree/master/docs/claimantmodel)
       description: [FT Claimant Model](./docs/design/README.md#claimant-model)
 - [X] Specify/document a system architecture for that model.
 - [X] Come up with some metadata format: [FirmwareMetadata](./api/firmware_metadata.go)
 - [X] Build a simple personality around that format.
 - [X] Extend personality to also store firmware images.
 - [X] Build a simple tool to create metadata given a "boot" image (e.g. Linux
     Kernel, WASM binary, etc.), and log it via the personality.
 - [X] Figure out a way to package the metadata with the bootable image.
 - [X] Build a noddy "device" emulator which enforces logging requirements, and refuses to boot
       an image unless all of the following are true:
    - [X] the metadata is present.
    - [X] the metadata has a "valid" signature
    - [X] the boot-image hash matches the one committed do in the metadata.
    - [X] a valid STH and inclusion proof for the metatdata is available and
          verifies correctly.
 - [X] Build a simple "flash" tool which refuses to flash an image to a device
       unless all of the boot-time requirements above are satisfied, in
       addition to requesting and validating a valid consistency proof between
       the previously seen STH and the new STH.
 - [X] Flash stores "proof bundle" on device for validation at boot time.
 - [X] Build simple monitor to tail the log and dump info from meta-data in realtime.
 - [X] Monitor is extended to validate firmware images hash

Planned future enhancements:
 - [ ] Integrate STH Witness support.
 - [ ] Add support for emulated and real hardware, e.g. via QEmu.

Running the Demo
----------------
In running this demo you will take on the role of several different actors within the ecosystem
to see how making firmware discoverable works in practice. You will then take the role of an
attacker trying to install malicious code onto a device, and see how the application of
transparency has made this attack much more expensive.

During the demo flow you will open several different terminals. It's far easier to use a terminal
that supports tabs in order to do this. The terminals simply make your life easier as different
logical actors have their own terminals in which to perform their actions.

Prerequisites:
* Install Docker and docker-compose
* Install Go (1.15+)
* Checkout:
  * [Trillian](https://github.com/google/trillian)
  * [Firmware Transparency (trillian-examples)](https://github.com/google/trillian-examples)

#### Terminal 1 - Trillian:
* Open terminal in root of `trillian` git repo, run:

```bash
export MYSQL_ROOT_PASSWORD="$(openssl rand -hex 16)"
docker-compose -f examples/deployment/docker-compose.yml up trillian-log-server trillian-log-signer
```

#### Terminal 1½ - Provision Log Tree:
* Run the following command to create a new tree inside Trillian, this only needs to be done once:

```bash
go run github.com/google/trillian/cmd/createtree --admin_server=localhost:8090
```

Record the tree ID that is returned by the command above, it will be referred to
as $TREE_ID by subsequent commands:

#### Terminal 2 - FT Personality:
* Open a terminal in the `binary_transparency/firmware` directory.
* A file is needed to hold the CAS DB which will back the log, this file
  needs to be available for the duration of this log, so writing to '/tmp'
  is considered risky. Choose a file path and add as below.

```bash
export CAS_DB_FILE='/full/path/to/file.db'
go run ./cmd/ft_personality/main.go --logtostderr -v=2 --tree_id=$TREE_ID --cas_db_file=${CAS_DB_FILE}
```

#### Terminal 3 - FT monitor
> The monitor "tails" the log, fetching each of the added entries and checking
> for inconsistencies in the structure and unexpected or malicious entries.

For our demo, we'll scan each firmware binary for the word `H4x0r3d`
and consider that any binary containing that string is a bad one.

* Open a terminal in the `binary_transparency/firmware` directory, and run
  the command below to start a monitor:

```bash
go run ./cmd/ft_monitor/ --logtostderr --keyword="H4x0r3d"
```

#### Terminal 4 - Firmware Vendor
The vendor is going to publish a new, legitimate, firmware now.

* cd to the root of `binary_transparency/firmware` for the following steps:
* We're going to log a new "firmware" build, as the vendor would:

```bash
go run cmd/publisher/publish.go --logtostderr --v=2 --timestamp="2020-10-10T15:30:20.10Z" --binary_path=./testdata/firmware/dummy_device/example.wasm --output_path=/tmp/update.ota --device=dummy
```

  This creates and submits a new firmware manifest to the log, waits for it to be
  included, and then builds a firmware update package ("OTA") and writes it out to local disk.

  > :mag_right: Very shortly you should see that the new firmware entry has
  > been spotted by the `FT monitor` above.
  >
  > This is important! If the `Firmware Vendor` is paying attention to the
  > contents of the log, they can check that every piece of firmware
  > they see logged there is expected and corresponds to a legitimate and
  > known-good build.  If they spot something _unexpected_ then they're
  > now aware that there is a problem which needs investigation...

#### Terminal 5 - Device owner
Through the power of scripted narrative, the owner of the target device now
has a firmware update to install (we'll re-use the `/tmp/update.ota` file created
in the last step).

1. cd to the root of `binary_transparency/firmware` for the following steps:

   Now that we have an update package for our new firmware, we can try flashing
   it to a device.

   > :frog: The repo contains a "dummy device" which uses the local disk
   > to store the device's state. You'll need to choose and create a directory
   > where this dummy device state will live - the instructions below assume
   > that is `/tmp/dummy_device', change the path if you're using something different.
   >
   > ```bash
   > mkdir /tmp/dummy_device
   > ```

   We'll use the `cmd/flash_tool` to do this flashing.

   > :warning: Note that the first time you do this the "dummy device" will
   > have no state and the flashing process will fail.
   > It will also fail if you've previously flashed firmware onto the device
   > from a different log.
   > In both of these cases, you can use the `--force` flag on the `flash_tool`.

   ```bash
   go run ./cmd/flash_tool/ --logtostderr --update_file=/tmp/update.ota --device_storage=/tmp/dummy_device  # --force if it's the first time
   ```

2. Boot the device.

   We'll boot the device emulator to check that everything is working ok.
   The "ROM" on the dummy device verifies the integrity of the firmware and
   proofs stored on the device.

    ```bash
    go run ./cmd/emulator/dummy --logtostderr --dummy_storage_dir=/tmp/dummy_device
    ```

> :frog: Because both the `flash_tool` and the device itself verifies the
> correctness of the inclusion proofs, they are convinced that the firmware
> is now _discoverable_ - anybody looking at the contents of the log _also_
> knows about its existence: this doesn't guarantee that the firmware is
> _"good"_, but we know at least that it can't be a covert targeted attack, _and_
> we can assume that the `Firmware vendor` is aware of it too.

#### Terminal 666 - The Hacker :shipit::computer:
_"Nice system you've got there. Let's test it..."_

* cd to the root of `binary_transparency/firmware` for the following steps.

:firecracker: The hacker has a malicious firmware they want to install on our
device. It's in `testdata/firmware/dummy_device/hacked.wasm`.

1. Write malicious firmware directly onto the device.

Let's imagine the hacker has access to our device, they're going to write their
malicious firmware directly over the top of our device's firmware:

```bash
cp testdata/firmware/dummy_device/hacked.wasm /tmp/dummy_device/firmware.bin
echo "mwuhahahaha :eyes:"
```

Let's watch as the device owner turns on their device in the next step...

#### Terminal 5 - Device owner
The device owner wants to use their device, however, unbeknownst to them it's
been HACKED!

Start up the device:

```bash
go run ./cmd/emulator/dummy --logtostderr --dummy_storage_dir=/tmp/dummy_device
```

We should see that the device refuses to boot, with an error similar to this:

```
dummy_emu.go:41] ROM: "failed to verify bundle: firmware measurement does not match metadata (0xefb19feba9ea0e0d5de73ac16d8aa9c4ceb092ecd13eab5548f49a61e85c367a2f2c8ce1eb36b67e1407148406705e67663dc5b6d3f05a45475f6e4a2b69e285 != 0xbf2f21936b66a0665883716ea4b1ceda609304ad76dd48f6423128bc36d4cb0fb5effaa9c1f2e328a5cfc25d2cb89a337d4285a8bc3e22dbb99bddbed19e7095)"
```

> :frog: This happened because the device _measured_ the firmware and compared that
> with what the firmware manifest claimed the expected measurement should be.
> Since the device firmware had been overwritten, they didn't match and the
> device refused to boot.
>
> Back to the drawing board, hacker!

#### Terminal 666 - The Hacker :shipit::computer:

We need to update the manifest on the device with the correct measurement for our hacked firmware.
Run the following command:

```bash
go run ./cmd/hacker/modify_bundle \
   --device dummy \
   --binary ./testdata/firmware/dummy_device/hacked.wasm \
   --input /tmp/dummy_device/bundle.json \
   --output /tmp/dummy_device/bundle.json \
   --sign=false
```

Let's watch as the device owner turns on their device in the next step...

#### Terminal 5 - Device owner

Start the device:

```bash
go run ./cmd/emulator/dummy --logtostderr --dummy_storage_dir=/tmp/dummy_device
```

and _again_, even though the device now gets past the firmware measurement check
we should see that the device still refuses to boot, with an error similar to:

```
F1215 17:47:06.449540 1724707 dummy_emu.go:41] ROM: "failed to verify bundle: failed to verify signature on FirmwareStatement: failed to verify signature crypto/rsa: verification error"
exit status 1
```

#### Terminal 666 - The Hacker :shipit::computer:

Ok, this time we'll "steal" the key and use it to sign our malicious firmware.
This is equivalent to the status-quo today.

Run the following command:

```bash
go run ./cmd/hacker/modify_bundle \
   --device dummy \
   --binary ./testdata/firmware/dummy_device/hacked.wasm \
   --input /tmp/dummy_device/bundle.json \
   --output /tmp/dummy_device/bundle.json \
   --sign=true
```

Let's watch as the device owner turns on their device in the next step...

#### Terminal 5 - Device owner

Start the device:

```bash
go run ./cmd/emulator/dummy --logtostderr --dummy_storage_dir=/tmp/dummy_device
```

We've got past the signature check, but now there's another error:

```
dummy_emu.go:41] ROM: "failed to verify bundle: invalid inclusion proof in bundle: calculated root:\n[202 202 214 35 92 129 74 43 92 63 27 232 69 79 93 26 187 86 24 174 32 49 53 19 122 252 252 241 139 226 122 79]\n does not match expected root:\n[186 61 229 40 73 60 245 168 87 2 6 107 225 25 186 169 85 12 74 158 126 168 255 168 27 149 245 138 27 211 67 234]"
exit status 1
```

> :frog: This message indicates that the device was unable to verify that the
> firmware manifest was included in the log using the inclusion proof and checkpoint
> which are present in the `bundle.json`.
> This makes sense - the modified firmware manifest _hasn't_ been included!
>
> Think again, hacker!

#### Terminal 666 - The Hacker :shipit::computer:

The *only* way we're going to get the hacked firmware onto the device
is to have a valid inclusion proof and checkpoint from the log which
covers the new firmware manifest.

Let's have the hacker break into the firmware vendor's offices and
add their modified manifest+firmware to the log...

```bash
go run cmd/publisher/publish.go --logtostderr --v=2 --timestamp="2020-10-10T23:00:00.00Z" --binary_path=./testdata/firmware/dummy_device/hacked.wasm --output_path=/tmp/bad_update.ota --device=dummy
```

> :frog: However, notice that the `FT Monitor` has spotted the firmware!
> Now the firmware vendor knows they have been compromised and can take action. :police:
>
> If the firmware vendor is complicit in the attack, then all is still not lost...
> Notice that the FT monitor has detected the malware in the new firmware too:
> ```
>    Malware detected matched pattern H4x0r3d
> ```
>
> Anybody else running a monitor also knows that malicious firmware has been
> logged and can raise the alarm.
