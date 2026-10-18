#!/usr/bin/env python3
"""Print SQL that loads the file store (offchain/dev-store) into one custody database.

  python3 offchain/custody/import.py dmt    offchain/dev-store | docker exec -i custody-dmt-db    psql -q -v ON_ERROR_STOP=1 -U dmt -d dmt
  python3 offchain/custody/import.py police offchain/dev-store | docker exec -i custody-police-db psql -q -v ON_ERROR_STOP=1 -U police -d police

dmt:    owner_record (the sealed envelopes), key_share (the DMT half) and owner_index
police: key_share (the Police half)

Bytes are copied as they are, so every anchor stays the one on the ledger;
eval/custody-verify.sh checks that. Existing rows are left alone, so re-running is harmless.

Afterwards move owner_record/, custody_dmt/ and custody_police/ off the portal host
(the store would otherwise still hold both key halves of every record); keep
registry-keys.json, which decides every vehicle handle.
"""
import base64
import json
import pathlib
import sys

org, store = sys.argv[1], pathlib.Path(sys.argv[2])
if org not in ('dmt', 'police'):
    sys.exit('org must be dmt or police')


def hexbytes(b64):
    return "'\\x" + base64.b64decode(b64).hex() + "'::bytea"


def quote(s):
    if not all(c in '0123456789abcdef' for c in s):
        raise ValueError(f'not a hex handle: {s!r}')
    return f"'{s}'"


print('BEGIN;')
if org == 'dmt':
    for f in sorted((store / 'owner_record').glob('*.json')):
        e = json.loads(f.read_text())
        print(f"INSERT INTO owner_record (vid, envelope_version, ciphertext, nonce, anchor_sha256) VALUES "
              f"({quote(e['vid'])}, {int(e['version'])}, {hexbytes(e['ciphertext'])}, {hexbytes(e['nonce'])}, {quote(e['anchor'])}) "
              f"ON CONFLICT (vid) DO NOTHING;")
    for f in sorted((store / 'custody_dmt').glob('*.json')):
        if f.name == 'owner_index.json':
            continue
        s = json.loads(f.read_text())
        print(f"INSERT INTO key_share (vid, share) VALUES ({quote(s['vid'])}, {hexbytes(s['share'])}) ON CONFLICT (vid) DO NOTHING;")
    index = store / 'custody_dmt' / 'owner_index.json'
    if index.exists():
        for vid, tag in sorted(json.loads(index.read_text()).get('vehicles', {}).items()):
            # Only vehicles whose sealed record exists: the index is keyed to owner_record.
            if (store / 'owner_record' / f'{vid}.json').exists():
                print(f"INSERT INTO owner_index (vid, owner_tag) VALUES ({quote(vid)}, {quote(tag)}) ON CONFLICT (vid) DO NOTHING;")
else:
    # Each Police half is stored with the anchor of the owner record it belongs to.
    for f in sorted((store / 'custody_police').glob('*.json')):
        s = json.loads(f.read_text())
        anchor = json.loads((store / 'owner_record' / f.name).read_text())['anchor']
        print(f"INSERT INTO key_share (vid, share, anchor_sha256) VALUES ({quote(s['vid'])}, {hexbytes(s['share'])}, {quote(anchor)}) ON CONFLICT (vid) DO NOTHING;")
print('COMMIT;')
