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

mkfs.ext4 ${o} -q -b 1024 -L firmware ${BLOCKS}
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
unset MNT

echo "Created image in ${o}:"
ls -lh ${o}
