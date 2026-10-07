#!/usr/bin/env bash
# air's entrypoint (.air.toml): run the anfra it just built as `anfra serve`, in the repo
# $ANFRA_DEV_REPO names. anfra serve uses its working directory as the repo.
set -euo pipefail
bin="$(pwd)/tmp/anfra"
cd "${ANFRA_DEV_REPO:?set ANFRA_DEV_REPO to a repo with Data Apps}"
exec "$bin" serve "$@"
