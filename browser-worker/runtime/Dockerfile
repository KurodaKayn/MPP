# 使用国内可用的镜像源替代官方 docker.io
FROM docker.m.daocloud.io/library/debian:bookworm-slim

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Switch to Tsinghua mirror (more stable for security blobs)
RUN sed -i 's/deb.debian.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list.d/debian.sources

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

# Pre-create X11 directory with correct permissions as root
RUN mkdir -p /tmp/.X11-unix && chmod 1777 /tmp/.X11-unix

# Set up environment
ENV DISPLAY=:99
ENV RESOLUTION=1366x768x24
ENV HOME=/home/browser

# Create a non-root user
RUN useradd -m browser
WORKDIR /home/browser

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
COPY proxy.py /proxy.py
RUN chmod +x /entrypoint.sh
RUN chown -R browser:browser /home/browser /proxy.py

# Switch to non-root user
USER browser

# Expose ports
EXPOSE 9222 6080

ENTRYPOINT ["/entrypoint.sh"]
