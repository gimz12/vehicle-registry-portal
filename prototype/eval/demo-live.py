#!/usr/bin/env python3
"""Seed the viva demo vehicles on the live network through the agency desk.

Every record goes through the same portal route an officer uses (/api/desk), so
handles, sealed owner records and the DMT owner index are produced exactly as
in live use. All people and vehicles are synthetic.

  S1  the "everything" vehicle: every record type the registry holds, ending
      under finance so a transfer can be refused live.
  R1–R3  three vehicles of one owner who repeats the same frauds: an odometer
      rollback on each, and claims with three different insurers that all use
      the same assessor and cite police reports (F6b serial claims, F6a
      serial rollback).

Customs, VET and insurer records also carry the agency's own internal fields
(importer, technician, policy, reserve), which the desk sends to that agency's
custody service after the commit; they never reach the ledger.

Usage: python3 eval/demo-live.py [base-url] [--backfill]   (portal and Fabric running)
A vehicle that already exists is skipped, so the script can be re-run.
--backfill: for vehicles that already exist, write their internal fields straight
to the Customs, VET and insurer services (tokens from offchain/custody/.env).
"""
import json
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request

ARGS = [a for a in sys.argv[1:] if not a.startswith('--')]
BACKFILL = '--backfill' in sys.argv
FAILURES = 0  # identity backfill steps that failed; a non-zero exit at the end
BASE = ARGS[0] if ARGS else 'http://localhost:3000'

# The agency's own internal fields per record id (all synthetic).
INTERNAL = {
    'CUSDEC/2026/D-0001': ('customs', {'declarant': 'Lanka Motor Imports (Pvt) Ltd (synthetic)', 'cusdecNo': 'CUSDEC/2026/D-0001', 'valuationLkr': 6850000}),
    'VET/ODO/D-1001': ('vet', {'technician': 'T. Fernando (synthetic)', 'certificateNo': 'VET/CERT/D-1001'}),
    'VET/EMS/D-1002': ('vet', {'technician': 'T. Fernando (synthetic)', 'certificateNo': 'VET/EMS/CERT/D-1002'}),
    'VET/ODO/D-1004': ('vet', {'technician': 'K. Silva (synthetic)', 'certificateNo': 'VET/CERT/D-1004'}),
    'CLM/A/D-5001': ('insurer', {'policyNo': 'POL/A/88123', 'claimFile': 'CLM/A/D-5001', 'assessorContact': 'ASR-1201, 077 000 1201 (synthetic)', 'reserveLkr': 420000}),
    'CLM/B/D-7002': ('insurer', {'policyNo': 'POL/B/55410', 'claimFile': 'CLM/B/D-7002', 'assessorContact': 'ASR-3302 (synthetic)', 'reserveLkr': 395000}),
}
for n in (1, 2, 3):
    # The same declarant on all three: visible to Customs inside its own records, never on the ledger.
    INTERNAL[f'CUSDEC/2026/R{n}-000{n}'] = ('customs', {'declarant': 'Panadura Auto Traders (synthetic)', 'cusdecNo': f'CUSDEC/2026/R{n}-000{n}', 'valuationLkr': 3100000 + n * 250000})
    INTERNAL[f'CLM/R{n}/0{n}'] = ('insurer', {'policyNo': f'POL/R{n}/7{n}0{n}', 'claimFile': f'CLM/R{n}/0{n}', 'assessorContact': 'ASR-7777 (synthetic)', 'reserveLkr': 510000 + n * 10000})
PORTS = {'customs': 8103, 'vet': 8104, 'insurer': 8105}

# What each demo vehicle is and its plate, kept in the DMT's database (synthetic). Older
# vehicles carry no description fingerprint, so the buyer page says "as recorded by the DMT".
IDENTITY = {
    'NZE161-3150001': ({'make': 'Toyota', 'model': 'Corolla Axio', 'year': 2016, 'engineCc': 1496, 'fuel': 'PETROL', 'colour': 'White'}, 'CAB-1234'),
    'RU1-4410071': ({'make': 'Honda', 'model': 'Vezel', 'year': 2017, 'engineCc': 1496, 'fuel': 'HYBRID', 'colour': 'Silver'}, 'KL-5678'),
    'MH34S-5520082': ({'make': 'Suzuki', 'model': 'Wagon R', 'year': 2018, 'engineCc': 658, 'fuel': 'PETROL', 'colour': 'Blue'}, 'WP-9012'),
    'ZE1-6630093': ({'make': 'Nissan', 'model': 'Leaf', 'year': 2019, 'engineCc': 0, 'fuel': 'ELECTRIC', 'colour': 'Grey'}, 'CBA-3456'),
}


def dmt_put(path, body):
    env = pathlib.Path(__file__).resolve().parent.parent / 'offchain' / 'custody' / '.env'
    tokens = dict(l.split('=', 1) for l in env.read_text().splitlines() if l.startswith('CUSTODY_TOKEN_'))
    req = urllib.request.Request(f'http://127.0.0.1:8101{path}', data=json.dumps(body).encode(), method='PUT',
                                 headers={'content-type': 'application/json', 'authorization': f"Bearer {tokens['CUSTODY_TOKEN_DMT']}"})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        raw = e.read() or b'{}'
        try:
            return e.code, json.loads(raw)
        except ValueError:
            return e.code, {'message': raw.decode(errors='replace')[:200]}


def identity_backfill(vid, chassis):
    """Store an existing vehicle's description and plate in the DMT's database (v1.8)."""
    global FAILURES
    desc, plate = IDENTITY[chassis]
    status, reply = dmt_put('/vehicles/description', {'vid': vid, **desc})
    FAILURES += status != 200
    print(f"  {'ok' if status == 200 else 'FAILED'}      description {desc['make']} {desc['model']} {desc['year']} ({status}{'' if status == 200 else ': ' + str(reply.get('message'))})")
    status, reply = dmt_put('/vehicles/plate', {'vid': vid, 'plate': plate})
    FAILURES += status != 200
    print(f"  {'ok' if status == 200 else 'FAILED'}      plate {plate} ({status}{'' if status == 200 else ': ' + str(reply.get('message'))})")


def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method,
                                 headers={'content-type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        raw = e.read() or b'{}'
        try:
            return e.code, json.loads(raw)
        except ValueError:
            return e.code, {'message': raw.decode(errors='replace')[:200]}


def desk(label, **fields):
    if fields.get('recordId') in INTERNAL:
        fields['internal'] = INTERNAL[fields['recordId']][1]
    status, reply = call('POST', '/api/desk', fields)
    text = reply.get('message') or reply.get('statusMessage') or reply
    print(f"  {'ok     ' if status == 200 else 'REFUSED'} {label}: {text}")
    if status != 200:
        raise SystemExit(f'stopped: {label} was refused')
    return reply


def handle(chassis):
    status, reply = call('POST', '/api/handle', {'chassis': chassis})
    assert status == 200, reply
    return reply['vid']


def backfill(vid, records):
    """Write existing vehicles' internal fields straight to each agency's own service."""
    env = pathlib.Path(__file__).resolve().parent.parent / 'offchain' / 'custody' / '.env'
    tokens = dict(l.split('=', 1) for l in env.read_text().splitlines() if l.startswith('CUSTODY_TOKEN_'))
    for rec in records:
        if rec not in INTERNAL:
            continue
        org, fields = INTERNAL[rec]
        req = urllib.request.Request(f'http://127.0.0.1:{PORTS[org]}/records/{urllib.parse.quote(rec, safe="")}',
                                     data=json.dumps({'vid': vid, 'fields': fields}).encode(), method='PUT',
                                     headers={'content-type': 'application/json', 'authorization': f"Bearer {tokens[f'CUSTODY_TOKEN_{org.upper()}']}"})
        with urllib.request.urlopen(req, timeout=30) as r:
            print(f"  ok      backfilled {rec} into the {org} service ({r.status})")


def exists(vid):
    status, _ = call('GET', f'/api/vehicle/{vid}?as=DMT')
    return status == 200


OWNER_A = {'name': 'Saman Kumara (synthetic)', 'nic': '199012345678', 'address': '22/4, Lake Road, Kurunegala'}
OWNER_B = {'name': 'Dilani Perera (synthetic)', 'nic': '198756789012', 'address': '9, Station Lane, Gampaha'}
SERIAL = {'name': 'Ruwan Jayasinghe (synthetic)', 'nic': '780123456V', 'address': '115, Galle Road, Panadura'}


def everything_vehicle():
    ch, en, mk = 'NZE161-3150001', '1NZ-8810001', 'Toyota'
    vid = handle(ch)
    print(f'\nS1 everything vehicle  chassis {ch}  handle {vid}')
    if exists(vid):
        print('  already on the ledger; skipped')
        if BACKFILL:
            backfill(vid, [k for k in INTERNAL if '/D-' in k])
            identity_backfill(vid, ch)
        return vid
    d, plate = IDENTITY[ch]
    spec = {k: d[k] for k in ('model', 'year', 'engineCc', 'fuel')}
    desk('Customs import clearance', agency='CUSTOMS', action='import', recordId='CUSDEC/2026/D-0001', chassisNumber=ch, engineNumber=en, make=mk, **spec)
    desk('DMT first registration (owner A)', agency='DMT', action='register', chassisNumber=ch, engineNumber=en, make=mk, provenance='CUSTOMS', owner=OWNER_A, plate=plate, colour=d['colour'], **spec)
    desk('VET odometer 42,150 km (day 30)', agency='VET', action='mileage', vid=vid, recordId='VET/ODO/D-1001', day=30, odometer=42150, vetLaneId='lane-3')
    desk('VET emission certificate (day 30)', agency='VET', action='emission', vid=vid, recordId='VET/EMS/D-1002', day=30, vetLaneId='lane-3')
    desk('VET odometer 58,900 km (day 395)', agency='VET', action='mileage', vid=vid, recordId='VET/ODO/D-1003', day=395, odometer=58900, vetLaneId='lane-1')
    desk('Police accident report (day 410)', agency='POLICE', action='accident', vid=vid, recordId='POL/ACC/2026/D-0101', day=410)
    desk('Claim, insurer A, cites the report, assessor ASR-1201', agency='INSURER', action='claim', vid=vid, recordId='CLM/A/D-5001', day=412, insurerId='insurer-a', evidenceAnchor='POL/ACC/2026/D-0101', evidenceKind='POLICE_REPORT', assessorId='ASR-1201')
    desk('Claim, insurer B, cites the SAME report (duplicate)', agency='INSURER', action='claim', vid=vid, recordId='CLM/B/D-7002', day=415, insurerId='insurer-b', evidenceAnchor='POL/ACC/2026/D-0101', evidenceKind='POLICE_REPORT', assessorId='ASR-3302')
    desk('Claim, insurer A, no evidence at all', agency='INSURER', action='claim', vid=vid, recordId='CLM/A/D-5009', day=520, insurerId='insurer-a')
    desk('DMT finance (absolute owner LOLC)', agency='DMT', action='encumber', vid=vid, recordId='FIN/D-0115', reference='AO/LOLC/2026/D-115')
    desk('DMT finance release (deletion letter)', agency='DMT', action='release', vid=vid, recordId='FIN/D-0116', reference='DL/LOLC/2026/D-88')
    desk('Police theft report (v1.8: recorded first)', agency='POLICE', action='report', vid=vid, recordId='POL/THF/2026/D-0200', reportRef='POL/THF/2026/D-0201')
    desk('Police theft flag, after the DMT confirms owner A reported it', agency='POLICE', action='theft', vid=vid, recordId='POL/THF/2026/D-0201', reportRef='POL/THF/2026/D-0201', reporterNic=OWNER_A['nic'])
    desk('Police clears the theft report (recovered)', agency='POLICE', action='clear', vid=vid, recordId='POL/THF/2026/D-0202')
    desk('DMT ownership transfer to owner B', agency='DMT', action='transfer', vid=vid, recordId='TRF/2026/D-0301', owner=OWNER_B)
    desk('VET odometer 51,300 km (day 760): ROLLBACK', agency='VET', action='mileage', vid=vid, recordId='VET/ODO/D-1004', day=760, odometer=51300, vetLaneId='lane-2')
    desk('DMT finance again (absolute owner People\'s Bank)', agency='DMT', action='encumber', vid=vid, recordId='FIN/D-0207', reference='AO/PB/2026/D-207')
    return vid


def serial_vehicle(n, ch, en, mk, insurer, readings, accident_day):
    vid = handle(ch)
    print(f'\nR{n} serial-fraud owner, vehicle {n}  chassis {ch}  handle {vid}')
    if exists(vid):
        print('  already on the ledger; skipped')
        if BACKFILL:
            backfill(vid, [f'CUSDEC/2026/R{n}-000{n}', f'CLM/R{n}/0{n}'])
            identity_backfill(vid, ch)
        return vid
    report = f'POL/ACC/2026/R{n}-0{n}01'
    d, plate = IDENTITY[ch]
    spec = {k: d[k] for k in ('model', 'year', 'engineCc', 'fuel')}
    desk('Customs import clearance', agency='CUSTOMS', action='import', recordId=f'CUSDEC/2026/R{n}-000{n}', chassisNumber=ch, engineNumber=en, make=mk, **spec)
    desk('DMT first registration (serial owner)', agency='DMT', action='register', chassisNumber=ch, engineNumber=en, make=mk, provenance='CUSTOMS', owner=SERIAL, plate=plate, colour=d['colour'], **spec)
    for i, (day, km) in enumerate(readings, 1):
        desk(f'VET odometer {km:,} km (day {day})', agency='VET', action='mileage', vid=vid, recordId=f'VET/ODO/R{n}-{i}', day=day, odometer=km, vetLaneId=f'lane-{i}')
    desk(f'Police accident report (day {accident_day})', agency='POLICE', action='accident', vid=vid, recordId=report, day=accident_day)
    desk(f'Claim, {insurer}, cites the report, assessor ASR-7777', agency='INSURER', action='claim', vid=vid, recordId=f'CLM/R{n}/0{n}', day=accident_day + 3, insurerId=insurer, evidenceAnchor=report, evidenceKind='POLICE_REPORT', assessorId='ASR-7777')
    return vid


s1 = everything_vehicle()
r = [
    serial_vehicle(1, 'RU1-4410071', 'L15B-2210071', 'Honda', 'insurer-a', [(20, 96500), (390, 61200)], 420),
    serial_vehicle(2, 'MH34S-5520082', 'R06A-3320082', 'Suzuki', 'insurer-b', [(40, 120400), (410, 74800)], 450),
    serial_vehicle(3, 'ZE1-6630093', 'EM57-4430093', 'Nissan', 'insurer-c', [(15, 88000), (380, 52300)], 470),
]

print('\nDemo links (portal):')
print(f'  S1 buyer view      {BASE}/vehicle/{s1}?as=BUYER')
print(f'  S1 DMT timeline    {BASE}/vehicle/{s1}?as=DMT')
print(f'  S1 insurer view    {BASE}/vehicle/{s1}?as=INSURER')
print(f'  S1 Police view     {BASE}/vehicle/{s1}?as=POLICE')
print(f'  R1 buyer view      {BASE}/vehicle/{r[0]}?as=BUYER')
print(f'  R1 Police case     {BASE}/police/{r[0]}')
for i, vid in enumerate(r, 1):
    print(f'  R{i} handle          {vid}')
print('Plates (agency desks only; buyers start from the handle):')
for ch, (d, plate) in IDENTITY.items():
    print(f"  {plate:<10} {d['make']} {d['model']} {d['year']} {d['colour']}  (chassis {ch})")
if FAILURES:
    print(f'{FAILURES} identity backfill step(s) FAILED')
    sys.exit(1)
