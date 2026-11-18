#!/bin/sh
# Container entrypoint: fetch any missing dictionary file into /data, then
# serve. Extra arguments go to the server (e.g. -port 8080).
#
# The server runs as the unprivileged yume user (uid 10001). Started as root,
# the default, this script first gives /data to that user: volumes written
# by images before 0.1.0 ran as root, and files restored with `docker cp`
# arrive root-owned. Started with --user, it skips that step.
# A failed download is retried here with a growing wait, so an offline first
# start says why in the log instead of crash-looping the container.
set -e
if [ "$(id -u)" = 0 ]; then
	if [ -n "$(find /data ! -user 10001 | head -n 1)" ]; then
		echo "yume: /data holds files from an older image or a restore; giving them to uid 10001"
		# -h: re-own a symlink itself, never the file it points to.
		chown -R -h 10001:10001 /data
	fi
	exec su-exec 10001:10001 /usr/local/bin/yume-entrypoint "$@"
fi
delay=15
until yume-server setup -data /data; do
	echo "dictionary download failed: EDRDG (www.edrdg.org) unreachable or sent a bad file, retrying in ${delay}s" >&2
	sleep "$delay"
	delay=$((delay * 2))
	if [ "$delay" -gt 300 ]; then
		delay=300
	fi
done
exec yume-server -bind 0.0.0.0 -data /data "$@"
