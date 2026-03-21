#!/bin/bash
set -euo pipefail
echo "clean-environment.sh: Cleaning up test compose projects"
docker compose ls --format json 2>/dev/null | python3 -c "
import json, sys
try:
    projects = json.load(sys.stdin)
    for p in projects:
        name = p.get('Name', '')
        if name.startswith('formae-test-'):
            print(name)
except:
    pass
" | while read -r name; do
    echo "  Removing project: $name"
    docker compose -p "$name" down -v --remove-orphans 2>/dev/null || true
done
echo "clean-environment.sh: Cleanup complete"
