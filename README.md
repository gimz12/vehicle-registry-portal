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

## Working on this repo

`main` is protected: direct pushes are rejected and changes arrive through a pull
request. After cloning, enable the local guard that stops you committing on `main`
by mistake:

```bash
git config core.hooksPath .githooks
```

Then the loop for any change is:

```bash
git switch -c my-change
git commit -m "..."
git push -u origin my-change
gh pr create --fill && gh pr merge --squash --delete-branch
```

## Author

S.N.I.W.M.K.V. Illankoon — NIBM / Coventry University
