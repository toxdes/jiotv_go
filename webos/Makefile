# JioTV Go - WebOS TV App
#
# Prerequisites:
#   npm install -g @webosose/ares-cli
#
# Usage:
#   make package          # Build .ipk package
#   make install          # Install to TV (requires TV_IP env var)
#   make launch           # Launch the app on TV
#   make inspect          # Open browser-based inspector on TV

TV_IP ?=

package:
	ares-package . -o ../

install: package
	@if [ -z "$(TV_IP)" ]; then echo "ERROR: Set TV_IP env var (e.g. TV_IP=192.168.1.200)"; exit 1; fi
	ares-install --device $(TV_IP) ../com.jiotv.go.tv_*.ipk

launch:
	@if [ -z "$(TV_IP)" ]; then echo "ERROR: Set TV_IP env var"; exit 1; fi
	ares-launch --device $(TV_IP) com.jiotv.go.tv

inspect:
	@if [ -z "$(TV_IP)" ]; then echo "ERROR: Set TV_IP env var"; exit 1; fi
	ares-inspect --device $(TV_IP) com.jiotv.go.tv

.PHONY: package install launch inspect
