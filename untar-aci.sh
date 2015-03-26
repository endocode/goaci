#!/usr/bin/bash -e

if [ -z "${1}" ]
then
	echo "Expected aci image as first parameter" >&2
	exit 1
fi

rm -rf ACI
mkdir ACI
tar -xzf "${1}" -C ACI
