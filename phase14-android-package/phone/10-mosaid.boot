#!/data/data/com.termux/files/usr/bin/sh
# Installed as ~/.termux/boot/10-mosaid by install-product.sh.
# Termux:Boot must be installed from the same source/signing family as Termux
# and opened once manually before BOOT_COMPLETED is delivered.
termux-wake-lock >/dev/null 2>&1 || true
. /data/data/com.termux/files/usr/etc/profile.d/start-services.sh
