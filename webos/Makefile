# JioTV Go - WebOS TV App
#
# Prerequisites:
#   npm install -g @webosose/ares-cli
#
# Usage:
#   make icons          # Generate all icon sizes from app_icon.png
#   make package        # Build .ipk package
#   make install        # Install to TV (uses ares-setup-device default or DEVICE= name)
#   make launch         # Launch the app on TV
#   make inspect        # Open browser-based inspector on TV

DEVICE ?= lgtv

icons:
	convert app_icon.png -resize 80x80 -type TrueColorAlpha PNG32:icon.png
	convert app_icon.png -resize 130x130 -type TrueColorAlpha PNG32:largeIcon.png
	convert app_icon.png -resize 300x300 -type TrueColorAlpha PNG32:splash.png
	convert app_icon.png -resize 300x300 -type TrueColorAlpha PNG32:../web/static/img/app_icon_splash.png

check:
	ares-package -c .

package:
	ares-package -e Makefile -e app_icon.png . -o ../

install: package
	ares-install --device $(DEVICE) ../*.ipk

launch:
	ares-launch --device $(DEVICE) com.toxdes.jiotvgo

inspect:
	ares-inspect --device $(DEVICE) com.toxdes.jiotvgo

.PHONY: icons check package install launch inspect
