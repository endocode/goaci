#!/usr/bin/bash -e

# Use: ./debug-aci.sh [-v] image.aci
# Produces an image-debug.aci which just sleeps
# Puts a bunch of utilities, so it is a bit usable for rkt entering it.

tmpdir="$(mktemp -d)"
acidir="${tmpdir}/aci"
rfsdir="${acidir}/rootfs"

verbose=0
if [ "${1}" = "-v" -o "${1}" = "--verbose" ]
then
    verbose=1
    image="${2}"
else
    image="${1}"
fi

if [ -z "${image}" ]
then
	echo "Expected aci image" >&2
	exit 1
fi

mkdir "${acidir}"
tar -xzf "${image}" -C "${acidir}"

echo 'prepare sleep forever app'
sfdir="${tmpdir}/sleep_forever"
mkdir "${sfdir}"

cat <<'EOF' >"${sfdir}/sleep.go"
package main

import "time"

func main() {
	for {
		time.Sleep(time.Hour)
	}
}
EOF

currdir=`pwd`
cd "${sfdir}"
go build .
cd "${currdir}"
cp "${sfdir}/sleep_forever" "${rfsdir}"

echo 'prepare manifest patcher'
mmdir="${tmpdir}/manman"
mkdir "${mmdir}"

cat <<'EOF' >"${mmdir}/mm.go"
package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
)

func die(err error) {
	fmt.Println(err)
	os.Exit(1)
}

func main() {
	json, err := ioutil.ReadFile("manifest")
	if err != nil {
		die(err)
	}
	im := schema.BlankImageManifest()
	if err := im.UnmarshalJSON(json); err != nil {
		die(err)
	}
	if im.App == nil {
		die(fmt.Errorf("Not an executable image"))
	}
	im.App.Exec = types.Exec{"/sleep_forever"}
	im.App.EventHandlers = []types.EventHandler{}
	json, err = im.MarshalJSON()
	if err != nil {
		die(err)
	}
	if err := ioutil.WriteFile("manifest", json, 0755); err != nil {
		die(err)
	}
}
EOF

cd "${mmdir}"
go build .
cd "${currdir}"
cp "${mmdir}/manman" "${acidir}"

function debug
{
	if [ $verbose -eq 1 ]
	then
		echo "$@"
	fi
}

function copy_bin_with_solibs
{
	local bin="${1}"
	debug "copy ${bin} to image"
	b="$(which --skip-alias --skip-functions "${bin}")"
	mkdir -p "${rfsdir}/$(dirname "${b}")"
	cp "${b}" "${rfsdir}/${b}"
	debug "copy solibs for ${bin}"
	for part in $(ldd "${b}")
	do
		lib=$(echo "${part}" | grep '/lib.*\.so' || true)
		acilib="${rfsdir}${lib}"
		if [ -n "${lib}" ]
		then
			debug "  ${lib}"
			if [ ! -e "${acilib}" ]
			then
				debug "    copying"
				cp "${lib}" "${acilib}"
			else
				debug "    already there"
			fi
		fi
	done
}

function copy_bins_with_solibs
{
	echo "copy $@ to aci"
	for bin in $@
	do
		copy_bin_with_solibs "$bin"
	done
}

copy_bins_with_solibs bash ls cat nano mkdir rmdir rm strace

# TODO: replace with manifest patcher
echo 'patching manifest'
cd "${acidir}"
./manman
rm -f ./manman
cd "${currdir}"

echo 'build the debug image'
filename="$(basename "${image}")"
extension="${filename##*.}"
filename="${filename%.*}-debug"
name="$(dirname "${image}")/${filename}.${extension}"

actool build --overwrite "${acidir}" "${name}"
rm -rf "${tmpdir}"
