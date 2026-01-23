#!/usr/bin/env bash

set -euo pipefail

# Ensure the Go toolchain can access the private SDK repository. If GOPRIVATE is
# already set, append the SDK repo if it's not present. Otherwise, initialize it.
if [[ -z "${GOPRIVATE:-}" ]]; then
  export GOPRIVATE="github.com/stainless-sdks/kernel-go"
elif [[ "$GOPRIVATE" != *"github.com/stainless-sdks/kernel-go"* ]]; then
  export GOPRIVATE="${GOPRIVATE},github.com/stainless-sdks/kernel-go"
fi

# Ensure the user's git configuration rewrites GitHub HTTPS URLs to SSH. This is
# required to clone private repositories via SSH without using a Github PAT.
if ! git config --global --get-all url."git@github.com:".insteadOf | grep -q "https://github.com/"; then
  echo "Your git configuration is missing the rewrite from HTTPS to SSH for GitHub repositories." >&2
  echo "Run the following command and try again:" >&2
  echo "  git config --global url.\"git@github.com:\".insteadOf \"https://github.com/\"" >&2
  exit 1
fi

# Ensure exactly one ref (commit hash or branch name) is provided
if [ "$#" -ne 1 ]; then
  echo "Usage: $(basename "$0") <commit-hash|branch-name>" >&2
  exit 1
fi
ref="$1"
commit=""
tmp_dir="/tmp/kernel-go"

# Clone the stainless-sdks/kernel-go repo at the provided commit (shallow clone for speed)
rm -rf "$tmp_dir"
git clone --filter=blob:none --quiet git@github.com:stainless-sdks/kernel-go "$tmp_dir"

# Determine the commit hash corresponding to the provided ref (commit hash or branch name)
pushd "$tmp_dir" >/dev/null

# If the ref looks like a commit SHA (7-40 hex chars), use it directly; otherwise treat it as a branch name
if [[ "$ref" =~ ^[0-9a-f]{7,40}$ ]]; then
  commit="$ref"
else
  # Fetch the branch (shallow for speed) and resolve its HEAD commit hash
  git fetch --depth=1 origin "$ref" >/dev/null 2>&1 || {
    echo "Error: failed to fetch branch '$ref' from remote repository." >&2
    popd >/dev/null
    exit 1
  }
  commit=$(git rev-parse FETCH_HEAD)
fi

# Compute the Go pseudo-version for the resolved commit
gomod_version=$(git show -s --abbrev=12 \
  --date=format:%Y%m%d%H%M%S \
  --format='v0.0.0-%cd-%h' "$commit")
popd >/dev/null

# Verify we're in the CLI module directory (go.mod must exist here)
if [ ! -f go.mod ]; then
  echo "go.mod not found in current directory. Please run this script from the CLI repository root (e.g. packages/cli)." >&2
  exit 1
fi

# Remove any existing replace directive for the SDK (ignore error if it doesn't exist)
# Then add the new replace directive pointing at the desired commit
go mod edit -dropreplace=github.com/kernel/kernel-go-sdk 2>/dev/null || true
go mod edit -replace=github.com/kernel/kernel-go-sdk=github.com/stainless-sdks/kernel-go@"$gomod_version"
go mod tidy

echo "go.mod updated to use github.com/stainless-sdks/kernel-go @ $gomod_version"
