#!/bin/bash
#
# build_log.sh is a script for building a small test log.

DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd)

export LOG=${DIR}/log
export SERVERLESS_LOG_PRIVATE_KEY="PRIVATE+KEY+astra+cad5a3d2+ASgwwenlc0uuYcdy7kI44pQvuz1fw8cS5NqS8RkZBXoy"
export SERVERLESS_LOG_PUBLIC_KEY="astra+cad5a3d2+AZJqeuyE/GnknsCNh1eCtDtwdAwKBddOlS8M2eI1Jt4b"

cd ${DIR}
rm -fr log

go run ../cmd/integrate --logtostderr --storage_dir=${LOG} --initialise

export LEAF=`mktemp`
for i in one two three four five six seven; do
  echo -n "$i" > ${LEAF}
  go run ../cmd/sequence --storage_dir=${LOG} --entries="${LEAF}"
  go run ../cmd/integrate --logtostderr --storage_dir=${LOG}
  size=$(sed -n '2 p' ${LOG}/checkpoint)
  cp ${LOG}/checkpoint ${LOG}/checkpoint.${size}
done

rm ${LEAF}
