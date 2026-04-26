#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: release-channel.sh <tag>}"

prerelease=false
make_latest=true
update_beta_latest=false
channel=stable

if [[ "$tag" == *-* ]]; then
  prerelease=true
  make_latest=false
  channel=prerelease
fi

if [[ "$tag" == v*-beta.* ]]; then
  update_beta_latest=true
  channel=beta
fi

cat <<EOF
prerelease=${prerelease}
make_latest=${make_latest}
update_beta_latest=${update_beta_latest}
channel=${channel}
EOF
