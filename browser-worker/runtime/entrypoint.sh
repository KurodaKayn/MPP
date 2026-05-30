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
# Ensure index.html exists for easy access
cp /usr/share/novnc/vnc.html /usr/share/novnc/index.html || true
websockify --web /usr/share/novnc/ $STREAM_PORT localhost:$VNC_PORT &

echo "Starting Chromium with CDP on port $CDP_PORT..."

# By passing the wrapper script to ensure arguments are parsed correctly
# EXTREMELY IMPORTANT: --remote-allow-origins=* must be set
/usr/lib/chromium/chromium \
    --no-sandbox \
    --disable-setuid-sandbox \
    --remote-debugging-address=0.0.0.0 \
    --remote-debugging-port=$CDP_PORT \
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
    "about:blank"
