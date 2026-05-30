#!/bin/bash
set -e

# Configuration
RESOLUTION=${RESOLUTION:-"1366x768x24"}
CDP_PORT=${CDP_PORT:-"9222"}
VNC_PORT=${VNC_PORT:-"5900"}
STREAM_PORT=${STREAM_PORT:-"6080"}
BROWSER_PROFILE="/tmp/browser-profile"

echo "Starting Xvfb with resolution $RESOLUTION..."
Xvfb :99 -screen 0 $RESOLUTION &
sleep 2

echo "Starting Openbox window manager..."
openbox &

echo "Starting x11vnc on port $VNC_PORT..."
x11vnc -display :99 -nopw -forever -localhost -rfbport $VNC_PORT &
sleep 1

echo "Starting websockify (noVNC) on port $STREAM_PORT..."
websockify --web /usr/share/novnc/ $STREAM_PORT localhost:$VNC_PORT &

echo "Starting Python CDP Proxy on port $CDP_PORT forwarding to 9223..."
# We use this proxy because Chromium ignores 0.0.0.0 and enforces Host checks.
# This makes it look like connections come from 127.0.0.1.
python3 /proxy.py &

echo "Starting Chromium with CDP on port 9223..."

# Use LOGIN_URL env var if provided, otherwise about:blank
TARGET_URL=${LOGIN_URL:-"about:blank"}

# By passing the wrapper script to ensure arguments are parsed correctly
# EXTREMELY IMPORTANT: --remote-allow-origins=* must be set
/usr/lib/chromium/chromium \
    --no-sandbox \
    --disable-setuid-sandbox \
    --remote-debugging-port=9223 \
    --remote-allow-origins=* \
    --user-data-dir=$BROWSER_PROFILE \
    --no-first-run \
    --no-default-browser-check \
    --disable-gpu \
    --disable-dev-shm-usage \
    --disable-web-security \
    --ignore-certificate-errors \
    --window-size=1366,768 \
    --window-position=0,0 \
    --kiosk \
    "$TARGET_URL"
