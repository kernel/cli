#!/bin/bash

set -e

if [ -z "$1" ]; then
    echo "Error: VERSION is required"
    echo "Usage: $0 <VERSION> [DESCRIPTION]"
    exit 1
fi

VERSION=$1
if [[ ! "$VERSION" =~ ^[0-9.]+$ ]]; then
    echo "Error: VERSION must contain only numbers and periods"
    echo "Usage: $0 <VERSION> [DESCRIPTION]"
    exit 1
fi
DESCRIPTION=${2:-"Version $VERSION"}

git tag -a v$VERSION -m "$DESCRIPTION"
git push origin v$VERSION
make release