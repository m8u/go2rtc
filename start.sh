#!/bin/bash

mkdir -p /mnt/recordings
mount -t cifs -o username=$SAMBA_USERNAME,password=$SAMBA_PASSWORD,iocharset=utf8 $SAMBA_PATH /mnt/recordings

go2rtc -config /config/go2rtc.yaml
