#!/bin/bash
#
# Checks that source files (.go and .proto) have the Apache License header.
# Automatically skips generated files.
set -eu

check_license() {
  local path="$1"

  if [[ "$path" =~ "usbarmory/bootloader" || "$path" =~ "omniwitness/usbarmory" ]]; then
    # This code forked from the USB Armory repo, and has
    # a LICENCE file in the directory.
    return 0
  fi

  if head -1 "$path" | grep -iq 'generated by'; then
    return 0
  fi

  # Look for "Apache License" on the file header
  if ! head -10 "$path" | grep -q 'Apache License'; then
    # Format: $path:$line:$message
    echo "$path:10:license header not found"
    return 1
  fi
}

main() {
  if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <path>"
    exit 1
  fi

  # Check USB Armory license is intact
  if ! grep -q "F-Secure" binary_transparency/firmware/devices/usbarmory/bootloader/LICENSE; then
    echo "LICENSE for forked USB armory missing"
    exit 1
  fi

  local code=0
  while [[ $# -gt 0 ]]; do
    local path="$1"
    if [[ -d "$path" ]]; then
      for f in "$path"/*.{go,proto}; do
        if [[ ! -f "$f" ]]; then
          continue  # Empty glob
        fi
        check_license "$f" || code=1
      done
    else
      check_license "$path" || code=1
    fi
    shift
  done
  exit $code
}

main "$@"
