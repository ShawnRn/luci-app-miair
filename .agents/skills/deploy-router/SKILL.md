---
name: deploy-router
description: >-
  Builds the luci-app-miair IPK package and deploys it directly to the OpenWrt router via SSH.
---

# Deploy luci-app-miair to Router

## Procedure

1. **Verify Go Core Tests**:
   ```bash
   cd core && go test -v ./...
   ```

2. **Build Linux ARM64 Binary & IPK Package**:
   ```bash
   ./scripts/build-packages.sh
   ```

3. **Deploy & Install on Router**:
   ```bash
   VERSION=$(tr -d '[:space:]' < VERSION)
   sshpass -p '20230323' scp -O -o StrictHostKeyChecking=no dist/luci-app-miair_${VERSION}-1_aarch64_cortex-a53.ipk root@192.168.10.1:/tmp/
   sshpass -p '20230323' ssh -o StrictHostKeyChecking=no root@192.168.10.1 "opkg install --force-reinstall /tmp/luci-app-miair_${VERSION}-1_aarch64_cortex-a53.ipk; /etc/init.d/miair restart; rm -f /tmp/luci-indexcache*"
   ```

4. **Verify Health**:
   ```bash
   sshpass -p '20230323' ssh -o StrictHostKeyChecking=no root@192.168.10.1 "cat /var/run/miair-status.json; logread | grep -i miair | tail -n 20"
   ```
