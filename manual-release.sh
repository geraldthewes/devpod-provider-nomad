#!/usr/bin/env bash
set -e

# Check if RELEASE_VERSION is provided
if [[ -z "$1" ]]; then
  echo "Usage: ./manual-release.sh <version>"
  echo "Example: ./manual-release.sh v0.1.0"
  exit 1
fi

RELEASE_VERSION=$1

# Validate version format (should start with v)
if [[ ! "$RELEASE_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+.*$ ]]; then
  echo "Error: Version must start with 'v' and follow semver format (e.g., v0.1.0)"
  exit 1
fi

echo "Building release for version: $RELEASE_VERSION"
echo ""

# Clean release directory
rm -rf release/
mkdir -p release/

# Build binaries
RELEASE_VERSION=$RELEASE_VERSION ./hack/build.sh

echo ""
echo "✓ Build complete!"
echo ""
echo "Built artifacts:"
ls -lh release/

echo ""
echo "============================================"
echo "Next steps:"
echo "============================================"
echo ""
echo "Option 1: Upload using GitHub CLI (recommended)"
echo "  gh release upload $RELEASE_VERSION release/*"
echo ""
echo "Option 2: Upload manually via web interface"
echo "  1. Go to: https://github.com/YOUR_USERNAME/devpod-provider-nomad/releases/tag/$RELEASE_VERSION"
echo "  2. Click 'Edit release'"
echo "  3. Drag and drop the files from the release/ directory"
echo "  4. Click 'Update release'"
echo ""
echo "Files to upload:"
find release/ -type f -exec basename {} \;
echo ""
