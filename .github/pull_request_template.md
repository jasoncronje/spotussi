<!-- The title becomes the squash commit's title: use a Conventional Commit, e.g. `fix(import): keep rows with a null URI`. -->

## What and why

## Checklist

- [ ] Every commit is signed off (`git commit -s`, see [CONTRIBUTING.md](../CONTRIBUTING.md#sign-your-commits-off-dco))
- [ ] `make lint` and `make test` pass
- [ ] API changes start in `api/openapi.yaml`, with `make gen-api` run
- [ ] UI changes checked in light and dark, desktop and phone (screenshots below)
- [ ] No real listening data, tokens, `.env` files or `data/` committed
