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

if ! command -v fakeroot >/dev/null 2>&1; then
	echo "fakeroot is required to create packages owned by root:root" >&2
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
Description: MiAir bridge for streaming AirPlay audio to Xiaomi speakers
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
IPK="$OUTPUT_DIR/${PACKAGE}_${VERSION}-${RELEASE}_${ARCH_IPK}.ipk"
rm -f "$IPK"
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
	--info "description:MiAir bridge for streaming AirPlay audio to Xiaomi speakers" \
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
	export APK APK_FILE PACKAGE VERSION RELEASE ARCH_APK
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
