# Vehicle Registry Portal

BSc (Hons) Computing final-year research project — a privacy-scoped consortium
blockchain vehicle ownership and lifecycle registry for Sri Lanka.

The registry keeps owner identities pseudonymous per vehicle, runs deterministic
cross-event fraud rules over that pseudonymous history, and **declares the checks
it cannot decide** under its operating privacy configuration rather than returning
a clean-looking record.

## Contents

| Folder | What's in it |
|---|---|
| `documents/` | The current thesis document (`Interim_Thesis.docx`) |
| `prototype/` | Buyer-facing pre-purchase disclosure portal (Nuxt 4 + Tailwind 4) |

The prototype will grow to include the Hyperledger Fabric chaincode, the shared Go
rule package, the synthetic-corpus generator and the evaluation harness — see
`prototype/README.md`.

## Running the portal

```bash
cd prototype
yarn install
yarn dev        # http://localhost:3000
```

## Author

S.N.I.W.M.K.V. Illankoon — NIBM / Coventry University
