# JioTV Go - WebOS TV App
#
# Prerequisites:
#   npm install -g @webosose/ares-cli
#
# Usage:
#   make package          # Build .ipk package
#   make install          # Install to TV (uses ares-setup-device default or DEVICE= name)
#   make launch           # Launch the app on TV
#   make inspect          # Open browser-based inspector on TV

DEVICE ?= lgtv

package:
	ares-package . -o ../

install: package
	ares-install --device $(DEVICE) ../com.jiotv.go.tv_*.ipk

launch:
	ares-launch --device $(DEVICE) com.jiotv.go.tv

inspect:
	ares-inspect --device $(DEVICE) com.jiotv.go.tv

.PHONY: package install launch inspect
