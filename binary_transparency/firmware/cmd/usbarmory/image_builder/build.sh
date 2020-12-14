# Copyright 2020 Google LLC. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# build.sh is a utility to create ext4 filesystem images for use in the
# firmware partition of SD Cards created for USB Armory devices.

#!/bin/bash
usage() {
    echo "Usage: $0 -u <path_to_unikernel> -o <output_file>" 1>&2
    exit 1
}

clean() {
    if [ ! -z "${MNT}" ]; then
    fusermount -u ${MNT}
    rmdir ${MNT}
    fi
}


while getopts ":u:o:f" opt; do
case ${opt} in
    u )
    u=${OPTARG}
    ;;
    o )
    o=${OPTARG}
    ;;
    f )
    f=1
    ;;
    * )
    usage
    ;;
esac
done
shift $((OPTIND - 1))

if [ -z "${u}" ] || [ -z "${o}" ]; then
    usage
fi

if [ -f  "${o}" ] && [ -z "${f}" ]; then
    echo "Output file ${o} already exists, use -f to force overwrite"
    if [ -z ${f} ]; then
        exit 2
    fi
fi

trap clean EXIT

set -e
MNT=$(mktemp --directory /tmp/build_image-XXXXX)

SIZE=$(stat --format='%s' ${u})
OVERHEAD=2560000
let BLOCKS=(${SIZE}+${OVERHEAD})/1024

mkfs.ext4 ${o} -q -b 1024 -O ^has_journal -L firmware ${BLOCKS}
tune2fs -O ^metadata_csum,^64bit ${o}
fuse2fs -o fakeroot ${o} ${MNT}

mkdir ${MNT}/boot
cp ${u} ${MNT}/boot/unikernel
h=$(sha256sum ${u} | awk '{print $1}')
sed -- "s/@HASH@/${h}/" > ${MNT}/boot/armory-boot.conf <<EOF
{
    "unikernel": [
        "/boot/unikernel",
        "@HASH@"
    ]
}
EOF

fusermount -u ${MNT}
tune2fs -O read-only ${o}
unset MNT

echo "Created image in ${o}:"
ls -lh ${o}
