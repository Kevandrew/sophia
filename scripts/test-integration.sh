#!/usr/bin/env bash

set -euo pipefail

go test -tags=integration ./internal/cli ./internal/service -count=1
