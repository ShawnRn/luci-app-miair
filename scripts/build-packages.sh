#!/bin/sh
set -eu

PROJECT_DIR=$(cd "$(dirname "$0")/.." && pwd)
VERSION=$(tr -d '[:space:]' < "$PROJECT_DIR/VERSION")
RELEASE=1
PACKAGE=luci-app-miair
ARCH_IPK=aarch64_cortex-a53
ARCH_APK=aarch64_cortex-a53
OUTPUT_DIR=${OUTPUT_DIR:-"$PROJECT_DIR/dist"}
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/miair-package.XXXXXX")

if ! command -v python3 >/dev/null 2>&1; then
	echo "python3 is required to create packages" >&2
	exit 1
fi

cleanup() {
	rm -rf "$BUILD_DIR"
}
trap cleanup EXIT INT TERM

case "$VERSION" in
	''|*[!0-9A-Za-z.+~-]*)
		echo "Invalid VERSION: $VERSION" >&2
		exit 1
		;;
esac

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
	"$CONTROL" \
	"$OUTPUT_DIR"

echo "Building miair-core for linux/arm64..."
(
	cd "$PROJECT_DIR/core"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath -ldflags="-s -w -X main.version=$VERSION" \
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

python3 - <<PYEOF
import os, sys, tarfile

def create_tar_gz(src_dir, output_path, arc_prefix="."):
    with tarfile.open(output_path, "w:gz", format=tarfile.GNU_FORMAT) as tar:
        for root, dirs, files in os.walk(src_dir):
            rel_dir = os.path.relpath(root, src_dir)
            if rel_dir == ".":
                target_dir = arc_prefix
            else:
                target_dir = os.path.normpath(os.path.join(arc_prefix, rel_dir))

            for d in sorted(dirs):
                full = os.path.join(root, d)
                arcname = os.path.normpath(os.path.join(target_dir, d))
                ti = tar.gettarinfo(full, arcname=arcname)
                ti.uid = 0
                ti.gid = 0
                ti.uname = "root"
                ti.gname = "root"
                tar.addfile(ti)

            for f in sorted(files):
                full = os.path.join(root, f)
                arcname = os.path.normpath(os.path.join(target_dir, f))
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

create_tar_gz(control_dir, os.path.join(build_dir, "control.tar.gz"), "./")
create_tar_gz(root_dir, os.path.join(build_dir, "data.tar.gz"), "./")

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
else
	echo "APK was not built: set APK=/path/to/apk-tools-v3" >&2
fi

(
	cd "$OUTPUT_DIR"
	shasum -a 256 "${PACKAGE}_${VERSION}-${RELEASE}_${ARCH_IPK}.ipk" \
		${APK:+"${PACKAGE}-${VERSION}-r${RELEASE}.${ARCH_APK}.apk"} \
		> SHA256SUMS
)
