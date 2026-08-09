# Vehicle Registry Prototype

BSc thesis prototype: privacy-scoped consortium blockchain vehicle ownership and
lifecycle registry for Sri Lanka (Hyperledger Fabric + Go), with this Nuxt app as
the buyer-facing pre-purchase disclosure portal.

Design source: `../../archive/thesis-source/Interim_Thesis.md` and `../../archive/REDESIGN_PLAN.md`
(`../documents/` holds the built thesis `.docx` only).
A static mockup of the portal page lives at `../../archive/buyer-portal-mockup.html`.

## Portal (this app)

```bash
yarn install
yarn dev        # http://localhost:3000
yarn lint       # eslint + typecheck
yarn test       # vitest
```

The portal is read-only and anonymous: one page per vehicle rendering the
chaincode's `GetVehicleHistory` output — checks passed / failed / **not
performed** (declared suppression + gap notices), mileage progression, and
evidence-tier badges. No auth, no database: the server route queries the Fabric
Gateway (to be wired up once the chaincode exists).

## Planned repo layout (see the approved plan)

- `rules/` — shared Go rule package (6 fraud rule families, Decide(), profiles)
- `chaincode/registry/` — Fabric chaincode importing `rules/`
- `cmd/vlfgen`, `cmd/harness`, `cmd/idsvc` — generator, sweep harness, pseudonymisation service
- `network/` — Fabric network configs (3 orgs → 5)
- `offchain/` — PostgreSQL docker-compose + schema
