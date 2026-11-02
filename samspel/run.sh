#!/bin/sh
# shairport-sync announces itself over mDNS through avahi, which needs a system D-Bus.
mkdir -p /run/dbus
dbus-uuidgen --ensure
dbus-daemon --system
# A fixed host name keeps this avahi from fighting over the name with the one on the Home Assistant host.
sed -i 's/^#\?host-name=.*/host-name=samspel/' /etc/avahi/avahi-daemon.conf
avahi-daemon --daemonize --no-drop-root
sleep 2
exec /hub -port 8080 -state /data/groups.json -air-dir /run/samspel
