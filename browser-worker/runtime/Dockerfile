FROM debian:bookworm-slim

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install Chromium, Xvfb, x11vnc, Openbox, and noVNC
RUN apt-get update && apt-get install -y \
    chromium \
    xvfb \
    x11vnc \
    openbox \
    websockify \
    novnc \
    procps \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Set up environment
ENV DISPLAY=:99
ENV RESOLUTION=1366x768x24
ENV HOME=/home/browser

# Create a non-root user for security
RUN useradd -m browser
WORKDIR /home/browser

# Copy the entrypoint script and make it executable
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Assign ownership to the browser user
RUN chown -R browser:browser /home/browser

# Switch to the non-root user
USER browser

# Expose ports: 
# 9222: Chrome DevTools Protocol (CDP)
# 6080: noVNC (HTTP/WebSocket)
EXPOSE 9222 6080

ENTRYPOINT ["/entrypoint.sh"]
