#!/usr/bin/env bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"

banner() {
  echo -e "\x1b[1m---[\x1b[7m$1\x1b[m]----------------------------------------------\x1b[0m"
}

cd ${INTEGRATION_DIR}/..

(
    banner "Build armory bootloader"
    cd devices/usbarmory/bootloader
    unset GOFLAGS
    make CROSS_COMPILE=arm-none-eabi- TARGET=usbarmory imx BOOT=uSD START=1024512
)

(
    banner "Build armory unikernel ext4 image"
    ./cmd/usbarmory/image_builder/build.sh -u ./testdata/firmware/usbarmory/example/tamago-example -o /tmp/armory.ext4 -f
)
