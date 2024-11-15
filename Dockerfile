# syntax=docker/dockerfile:labs

# 0. Prepare images
ARG PYTHON_VERSION="3.11"
ARG GO_VERSION="1.22"
ARG NGROK_VERSION="3"

FROM python:${PYTHON_VERSION}-alpine AS base
FROM ngrok/ngrok:${NGROK_VERSION}-alpine AS ngrok


# 1. Build go2rtc binary
FROM --platform=linux/amd64 golang:${GO_VERSION}-alpine AS build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

RUN apk add git

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. Collect all files
FROM scratch AS rootfs

COPY --from=build /build/go2rtc /usr/local/bin/
COPY --from=ngrok /bin/ngrok /usr/local/bin/


# 3. Final image
FROM base

# Install ffmpeg, tini (for signal handling),
# and other common tools for the echo source.
# alsa-plugins-pulse for ALSA support (+0MB)
# font-droid for FFmpeg drawtext filter (+2MB)
# samba-client for recording to a Samba server (+2.7MB)
# cifs-utils for mounting Samba folder as network drive (+144KB)
RUN apk add --no-cache tini ffmpeg bash curl jq alsa-plugins-pulse font-droid samba-client cifs-utils

# Hardware Acceleration for Intel CPU (+50MB)
ARG TARGETARCH

RUN if [ "${TARGETARCH}" = "amd64" ]; then apk add --no-cache libva-intel-driver intel-media-driver; fi

# Hardware: AMD and NVidia VAAPI (not sure about this)
# RUN libva-glx mesa-va-gallium
# Hardware: AMD and NVidia VDPAU (not sure about this)
# RUN libva-vdpau-driver mesa-vdpau-gallium (+150MB total)

COPY --from=rootfs / /
COPY scripts /scripts
COPY start.sh /start.sh

ENTRYPOINT ["/sbin/tini", "--"]
WORKDIR /config

CMD ["/bin/sh", "/start.sh"]
