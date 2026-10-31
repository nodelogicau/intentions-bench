#!/bin/sh
# Builds the seed workspaces under scenarios/seeds with the reference CLI.
# Every object is written as the person's own act (an author, no harness),
# at a fixed time a week before the scenarios' `now`. Imported commitments
# have no verb, so they are written as files and stamped with their version.
#
# Usage: scripts/build-seeds.sh [intentions-binary]
set -eu

CLI=${1:-intentions}
ROOT=$(cd "$(dirname "$0")/.." && pwd)/scenarios/seeds
ADA=https://example.com/people/ada
PRIYA=https://example.com/people/priya
VENDOR=https://example.com/people/northwind
export INTENTIONS_AUTHOR=$ADA
export INTENTIONS_NOW=2026-10-05T09:00:00+11:00
unset INTENTIONS_HARNESS INTENTIONS_MODEL INTENTIONS_WORKSPACE

id() { python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'; }

# init <dir>: a workspace with one firm terminus and weekday working hours.
init() {
	rm -rf "$ROOT/$1"
	"$CLI" init "$ROOT/$1" --subject $ADA --timezone Australia/Melbourne --hemisphere south >/dev/null
	cd "$ROOT/$1"
	"$CLI" availability add --subject $ADA --title "Weekday working hours" --capacity PT8H \
		--calendar 2026-W41/2026-W46 --clock 09:00/17:00 --cadence "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" \
		--valid-until 2026-W46 --json >/dev/null
}

terminus() {
	"$CLI" intention add --title "Being someone who follows through" --stability firm --json | id
}

# commitment <id> <title> <start> <duration> <party-uri> <party-status> <uid>
# writes an imported commitment in which Ada's answer is still tentative.
commitment() {
	cat >"commitments/$1.yaml" <<EOF
id: $1
parties:
  - uri: $ADA
    status: tentative
  - uri: $5
    status: $6
placement:
  start: $3
  duration: $4
origin: import
external:
  system: icalendar
  uid: $7
title: $2
source:
  author: $ADA
timestamp: 2026-10-04T22:00:00Z
acknowledgements: []
EOF
	v=$("$CLI" version-of "$1" --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')
	python3 - "commitments/$1.yaml" "$v" <<'EOF'
import sys
p, v = sys.argv[1], sys.argv[2]
lines = open(p).read().splitlines(True)
open(p, "w").write(lines[0] + "version: " + v + "\n" + "".join(lines[1:]))
EOF
}

# newid <prefix>: a prefixed UUIDv7 minted at INTENTIONS_NOW.
newid() {
	python3 - "$1" "$INTENTIONS_NOW" <<'PY'
import datetime, os, sys, uuid
ms = int(datetime.datetime.fromisoformat(sys.argv[2]).timestamp() * 1000)
b = bytearray(ms.to_bytes(6, "big") + os.urandom(10))
b[6] = 0x70 | (b[6] & 0x0F)
b[8] = 0x80 | (b[8] & 0x3F)
print(sys.argv[1] + "_" + str(uuid.UUID(bytes=bytes(b))))
PY
}

# An empty workspace with one firm terminus.
init one-firm-terminus
T=$(terminus)
printf 'follow-through: %s\n' "$T" >keys.yaml
"$CLI" index >/dev/null

# A firm auto_select policy (itself a terminus) and a short unplaced
# intention it covers.
init auto-select-policy
P=$("$CLI" intention add --title "Being someone who keeps small things moving" --stability firm \
	--auto-select max_duration=PT1H --json | id)
S=$("$CLI" intention add --title "Stretching session" --duration PT30M --calendar 2026-W42 \
	--serves "$P:for-the-sake-of" --json | id)
printf 'small-things: %s\nstretch: %s\n' "$P" "$S" >keys.yaml
"$CLI" index >/dev/null

# An imported invitation Ada has not answered.
init pending-invitation
T=$(terminus)
C=$(newid cmt)
commitment "$C" "Quarterly review with Priya" 2026-10-15T14:00:00+11:00 PT1H $PRIYA accepted review-20261015@example.com
printf 'follow-through: %s\nreview: %s\n' "$T" "$C" >keys.yaml
"$CLI" index >/dev/null

# A placed intention, and an imported commitment that clashes with it.
init placed-clash
T=$(terminus)
G=$("$CLI" intention add --title "Deep work on the grant application" --duration PT2H \
	--calendar 2026-10-14 --clock 09:00/12:00 --serves "$T:for-the-sake-of" --json | id)
INTENTIONS_NOW=2026-10-12T08:00:00+11:00 "$CLI" select "$G" --candidate 1 --json >/dev/null
C=$(newid cmt)
commitment "$C" "Northwind product demo" 2026-10-14T10:00:00+11:00 PT1H $VENDOR accepted demo-20261014@northwind.example
printf 'follow-through: %s\ngrant: %s\ndemo: %s\n' "$T" "$G" "$C" >keys.yaml
"$CLI" index >/dev/null

for d in one-firm-terminus auto-select-policy pending-invitation placed-clash; do
	"$CLI" validate --workspace "$ROOT/$d" --json >/dev/null || { echo "seed $d does not validate" >&2; exit 1; }
done
echo "seeds written under $ROOT"
