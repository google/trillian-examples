# http://github.com/usbarmory/tamago-example
#
# Copyright (c) F-Secure Corporation
# https://foundry.f-secure.com
#
# Use of this source code is governed by the license
# that can be found in the LICENSE file.

BUILD_USER ?= $(shell whoami)
BUILD_HOST ?= $(shell hostname)
BUILD_DATE ?= $(shell /bin/date -u "+%Y-%m-%d %H:%M:%S")
BUILD = ${BUILD_USER}@${BUILD_HOST} on ${BUILD_DATE}
REV = $(shell git rev-parse --short HEAD 2> /dev/null)

SHELL = /bin/bash

APP := omniwitness
TARGET ?= "usbarmory"
GOENV := GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 GOOS=tamago GOARM=7 GOARCH=arm
TEXT_START := 0x80010000 # ramStart (defined in imx6/imx6ul/memory.go) + 0x10000
GOFLAGS := -tags ${TARGET} -trimpath -ldflags "-s -w -T $(TEXT_START) -E _rt0_arm_tamago -R 0x1000 -X 'main.Build=${BUILD}' -X 'main.Revision=${REV}'"
QEMU ?= qemu-system-arm -machine mcimx6ul-evk -cpu cortex-a7 -m 512M \
        -nographic -monitor none -serial null -serial stdio -net none \
        -semihosting -d unimp

.PHONY: clean qemu qemu-gdb

#### primary targets ####

all: $(APP)

imx: $(APP).imx

imx_signed: $(APP)-signed.imx

elf: $(APP)

#### utilities ####

check_tamago:
	@if [ "${TAMAGO}" == "" ] || [ ! -f "${TAMAGO}" ]; then \
		echo 'You need to set the TAMAGO variable to a compiled version of https://github.com/usbarmory/tamago-go'; \
		exit 1; \
	fi

check_hab_keys:
	@if [ "${HAB_KEYS}" == "" ]; then \
		echo 'You need to set the HAB_KEYS variable to the path of secure boot keys'; \
		echo 'See https://github.com/usbarmory/usbarmory/wiki/Secure-boot-(Mk-II)'; \
		exit 1; \
	fi

dcd:
	@if test "${TARGET}" = "usbarmory"; then \
		cp -f $(GOMODCACHE)/$(TAMAGO_PKG)/board/usbarmory/m2/imximage.cfg $(APP).dcd; \
	elif test "${TARGET}" = "mx6ullevk"; then \
		cp -f $(GOMODCACHE)/$(TAMAGO_PKG)/board/nxp/mx6ullevk/imximage.cfg $(APP).dcd; \
	else \
		echo "invalid target - options are: usbarmory, mx6ullevk"; \
		exit 1; \
	fi

clean:
	rm -f $(APP)
	@rm -fr $(APP).bin $(APP).imx $(APP)-signed.imx $(APP).csf $(APP).dcd

qemu: $(APP)
	$(QEMU) -kernel $(APP)

qemu-gdb: GOFLAGS := $(GOFLAGS:-s=)
qemu-gdb: GOFLAGS := $(GOFLAGS:-w=)
qemu-gdb: $(APP)
	$(QEMU) -kernel $(APP) -S -s

#### dependencies ####

$(APP): check_tamago
	$(GOENV) $(TAMAGO) build $(GOFLAGS) -o ${APP}

$(APP).dcd: check_tamago
$(APP).dcd: GOMODCACHE=$(shell ${TAMAGO} env GOMODCACHE)
$(APP).dcd: TAMAGO_PKG=$(shell grep "github.com/usbarmory/tamago v" ../../../../go.mod | awk '{print $$1"@"$$2}')
$(APP).dcd: dcd

$(APP).bin: CROSS_COMPILE=arm-none-eabi-
$(APP).bin: $(APP)
	$(CROSS_COMPILE)objcopy -j .text -j .rodata -j .shstrtab -j .typelink \
	    -j .itablink -j .gopclntab -j .go.buildinfo -j .noptrdata -j .data \
	    -j .bss --set-section-flags .bss=alloc,load,contents \
	    -j .noptrbss --set-section-flags .noptrbss=alloc,load,contents \
	    $(APP) -O binary $(APP).bin

$(APP).imx: $(APP).bin $(APP).dcd
	mkimage -n $(APP).dcd -T imximage -e $(TEXT_START) -d $(APP).bin $(APP).imx
	# Copy entry point from ELF file
	dd if=$(APP) of=$(APP).imx bs=1 count=4 skip=24 seek=4 conv=notrunc

