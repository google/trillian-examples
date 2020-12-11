#!/usr/bin/env bash
set -e
. $(go list -f '{{.Dir}}' github.com/google/trillian)/integration/functions.sh
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/ft_functions.sh

cd binary_transparency/firmware

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
