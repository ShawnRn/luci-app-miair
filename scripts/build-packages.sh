#!/bin/sh
set -eu

PROJECT_DIR=$(cd "$(dirname "$0")/.." && pwd)
VERSION=$(tr -d '[:space:]' < "$PROJECT_DIR/VERSION")
RELEASE=1
PACKAGE=luci-app-miair
OUTPUT_DIR=${OUTPUT_DIR:-"$PROJECT_DIR/dist"}

case "$VERSION" in
	''|*[!0-9A-Za-z.+~-]*)
		echo "Invalid VERSION: $VERSION" >&2
		exit 1
		;;
esac

mkdir -p "$OUTPUT_DIR"

# Target definitions: ARCH_IPK:ARCH_APK:GOARCH:GOARM:GOMIPS
ALL_TARGETS="
aarch64_cortex-a53:aarch64:arm64::
aarch64_generic:aarch64:arm64::
x86_64:x86_64:amd64::
arm_cortex-a7_neon-vfpv4:armv7:arm:7:
arm_cortex-a9:armv7:arm:7:
arm_cortex-a15_neon-vfpv4:armv7:arm:7:
mipsel_24kc:mipsel:mipsle::softfloat
mips_24kc:mips:mips::softfloat
"

REQUESTED_ARCH="${1:-all}"
if [ "$REQUESTED_ARCH" != "all" ]; then
	MATCHED=""
	for entry in $ALL_TARGETS; do
		arch=$(echo "$entry" | cut -d: -f1)
		if [ "$arch" = "$REQUESTED_ARCH" ]; then
			MATCHED="$entry"
			break
		fi
	done
	if [ -z "$MATCHED" ]; then
		echo "Unknown architecture: $REQUESTED_ARCH" >&2
		echo "Available architectures:" >&2
		for entry in $ALL_TARGETS; do
			echo "  - $(echo "$entry" | cut -d: -f1)" >&2
		done
		exit 1
	fi
	TARGET_LIST="$MATCHED"
else
	TARGET_LIST="$ALL_TARGETS"
fi

build_single_arch() {
	ARCH_IPK="$1"
	ARCH_APK="$2"
	GOARCH="$3"
	GOARM="$4"
	GOMIPS="$5"

	BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/miair-package.XXXXXX")
	ROOT="$BUILD_DIR/root"
	CONTROL="$BUILD_DIR/control"
	mkdir -p \
		"$ROOT/etc/config" \
		"$ROOT/etc/init.d" \
		"$ROOT/usr/bin" \
		"$ROOT/usr/lib/lua/luci/controller" \
		"$ROOT/usr/lib/lua/luci/model/cbi/miair" \
		"$ROOT/usr/lib/lua/luci/view/miair" \
		"$ROOT/usr/share/miair" \
		"$CONTROL"

	echo "==> Building miair-core for $ARCH_IPK (GOARCH=$GOARCH GOARM=$GOARM GOMIPS=$GOMIPS)..."
	(
		cd "$PROJECT_DIR/core"
		export CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH"
		if [ -n "$GOARM" ]; then
			export GOARM="$GOARM"
		fi
		if [ -n "$GOMIPS" ]; then
			export GOMIPS="$GOMIPS"
		fi
		go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
			-o "$ROOT/usr/bin/miair-core" .
	)

	cp "$PROJECT_DIR/root/etc/config/miair" "$ROOT/etc/config/miair"
	cp "$PROJECT_DIR/root/etc/init.d/miair" "$ROOT/etc/init.d/miair"
	cp "$PROJECT_DIR/luasrc/controller/miair.lua" "$ROOT/usr/lib/lua/luci/controller/miair.lua"
	cp "$PROJECT_DIR/luasrc/model/cbi/miair/miair.lua" "$ROOT/usr/lib/lua/luci/model/cbi/miair/miair.lua"
	cp "$PROJECT_DIR/luasrc/view/miair/status.htm" "$ROOT/usr/lib/lua/luci/view/miair/status.htm"
	cp "$PROJECT_DIR/VERSION" "$ROOT/usr/share/miair/version"
	chmod 0755 "$ROOT/usr/bin/miair-core" "$ROOT/etc/init.d/miair"
	chmod 0644 "$ROOT/etc/config/miair" "$ROOT/usr/share/miair/version"

	INSTALLED_SIZE=$(du -sk "$ROOT" | awk '{print $1 * 1024}')
	cat > "$CONTROL/control" <<EOF
Package: $PACKAGE
Version: $VERSION-$RELEASE
Architecture: $ARCH_IPK
Maintainer: Shawn Rain <https://github.com/ShawnRn>
Depends: libc, luci-base, ca-bundle
Section: luci
Priority: optional
Installed-Size: $INSTALLED_SIZE
Homepage: https://github.com/ShawnRn/luci-app-miair
Description: MiAir AirPlay and DLNA bridge for Xiaomi speakers
EOF

	cat > "$CONTROL/conffiles" <<'EOF'
/etc/config/miair
EOF

	cat > "$CONTROL/postinst" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] || {
	/etc/init.d/miair enable
	/etc/init.d/miair restart
	rm -f /tmp/luci-indexcache /tmp/luci-indexcache.*
	/etc/init.d/rpcd restart
}
exit 0
EOF

	cat > "$CONTROL/prerm" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] || {
	/etc/init.d/miair stop
	/etc/init.d/miair disable
}
exit 0
EOF
	chmod 0755 "$CONTROL/postinst" "$CONTROL/prerm"

	IPK="$OUTPUT_DIR/${PACKAGE}_${VERSION}-${RELEASE}_${ARCH_IPK}.ipk"
	printf '2.0\n' > "$BUILD_DIR/debian-binary"
	rm -f "$IPK"

	if command -v fakeroot >/dev/null 2>&1; then
		cat > "$BUILD_DIR/make-tars.sh" <<'EOF'
#!/bin/sh
set -eu
chown -R 0:0 "$CONTROL" "$ROOT" "$BUILD_DIR/debian-binary"
(cd "$CONTROL" && tar -czf "$BUILD_DIR/control.tar.gz" .)
(cd "$ROOT" && tar -czf "$BUILD_DIR/data.tar.gz" .)
(cd "$BUILD_DIR" && tar -czf "$IPK" ./debian-binary ./data.tar.gz ./control.tar.gz)
EOF
		chmod 0755 "$BUILD_DIR/make-tars.sh"
		export CONTROL ROOT BUILD_DIR IPK
		fakeroot "$BUILD_DIR/make-tars.sh"
	else
		python3 - <<PYEOF
import os, sys, tarfile

def create_tar_gz(src_dir, output_path):
    with tarfile.open(output_path, "w:gz", format=tarfile.GNU_FORMAT) as tar:
        for root, dirs, files in os.walk(src_dir):
            rel_dir = os.path.relpath(root, src_dir)
            target_dir = "." if rel_dir == "." else "./" + rel_dir

            for d in sorted(dirs):
                full = os.path.join(root, d)
                arcname = target_dir + "/" + d if target_dir != "." else "./" + d
                ti = tar.gettarinfo(full, arcname=arcname)
                ti.uid = 0
                ti.gid = 0
                ti.uname = "root"
                ti.gname = "root"
                tar.addfile(ti)

            for f in sorted(files):
                full = os.path.join(root, f)
                arcname = target_dir + "/" + f if target_dir != "." else "./" + f
                ti = tar.gettarinfo(full, arcname=arcname)
                ti.uid = 0
                ti.gid = 0
                ti.uname = "root"
                ti.gname = "root"
                with open(full, "rb") as fp:
                    tar.addfile(ti, fp)

build_dir = "$BUILD_DIR"
control_dir = "$CONTROL"
root_dir = "$ROOT"
ipk_path = "$IPK"

create_tar_gz(control_dir, os.path.join(build_dir, "control.tar.gz"))
create_tar_gz(root_dir, os.path.join(build_dir, "data.tar.gz"))

with tarfile.open(ipk_path, "w:gz", format=tarfile.GNU_FORMAT) as tar:
    for name in ["debian-binary", "data.tar.gz", "control.tar.gz"]:
        full = os.path.join(build_dir, name)
        ti = tar.gettarinfo(full, arcname="./" + name)
        ti.uid = 0
        ti.gid = 0
        ti.uname = "root"
        ti.gname = "root"
        with open(full, "rb") as fp:
            tar.addfile(ti, fp)
PYEOF
	fi

	echo "Created $IPK"

	if [ -n "${APK:-}" ]; then
		APK_FILE="$OUTPUT_DIR/${PACKAGE}-${VERSION}-r${RELEASE}.${ARCH_APK}.apk"
		rm -f "$APK_FILE"
		cat > "$BUILD_DIR/make-apk.sh" <<'EOF'
#!/bin/sh
set -eu
chown -R 0:0 "$ROOT"
exec "$APK" mkpkg \
	--xattrs=no \
	--info "name:$PACKAGE" \
	--info "version:$VERSION-r$RELEASE" \
	--info "description:MiAir AirPlay and DLNA bridge for Xiaomi speakers" \
	--info "arch:$ARCH_APK" \
	--info "license:custom" \
	--info "origin:$PACKAGE" \
	--info "url:https://github.com/ShawnRn/luci-app-miair" \
	--info "maintainer:Shawn Rain <https://github.com/ShawnRn>" \
	--info "depends:libc luci-base ca-bundle" \
	--script "post-install:$CONTROL/postinst" \
	--script "pre-deinstall:$CONTROL/prerm" \
	--files "$ROOT" \
	--output "$APK_FILE"
EOF
		chmod 0755 "$BUILD_DIR/make-apk.sh"
		export APK APK_FILE PACKAGE VERSION RELEASE ARCH_APK ROOT CONTROL
		fakeroot "$BUILD_DIR/make-apk.sh"
		echo "Created $APK_FILE"
	fi

	rm -rf "$BUILD_DIR"
}

for target in $TARGET_LIST; do
	arch_ipk=$(echo "$target" | cut -d: -f1)
	arch_apk=$(echo "$target" | cut -d: -f2)
	goarch=$(echo "$target" | cut -d: -f3)
	goarm=$(echo "$target" | cut -d: -f4)
	gomips=$(echo "$target" | cut -d: -f5)

	build_single_arch "$arch_ipk" "$arch_apk" "$goarch" "$goarm" "$gomips"
done

echo "==> Generating SHA256 checksums..."
(
	cd "$OUTPUT_DIR"
	rm -f SHA256SUMS
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -- *.ipk ${APK:+*.apk} > SHA256SUMS 2>/dev/null || sha256sum -- *.ipk > SHA256SUMS
	else
		shasum -a 256 -- *.ipk ${APK:+*.apk} > SHA256SUMS 2>/dev/null || shasum -a 256 -- *.ipk > SHA256SUMS
	fi
)
echo "Packages built successfully in $OUTPUT_DIR"
