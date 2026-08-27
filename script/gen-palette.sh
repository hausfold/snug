#!/usr/bin/env bash
# Regenerate palette.go from a nebelung checkout.
#
#   script/gen-palette.sh ~/code/workshop/nebelung
#
# snug cannot take nebelung as a dependency — nebelung is a Whiskers/Tera
# generator with no Go surface — so the hex values are VENDORED, and this script
# is the only thing allowed to write them. Hand-editing palette.go puts the
# family's CLIs back where they were: seven hand-picked 256-colour indices, ΔE
# 2–27 from the flavour every other tool on the machine is wearing.
set -euo pipefail
neb="${1:-$HOME/code/workshop/nebelung}"
[ -d "$neb/palette" ] || { echo "no nebelung palette at $neb/palette" >&2; exit 1; }
exec python3 script/gen-palette.py "$neb" > palette.go
