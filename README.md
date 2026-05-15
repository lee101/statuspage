# statuspage.app.nz

A simple Go/fasthttp marketing site for `statuspage.app.nz` by Applied AI NZ.

## Run

```sh
cp .env.example .env
go run .
```

The server listens on `PORT` or `8080`.

## Postgres

For local development:

```sh
sudo -u postgres psql -f scripts/setup_postgres.sql
go run .
```

The app reads `DATABASE_URL` and runs `migrations/001_init.sql` on startup.

## Build and test

```sh
make build
make test
```

`make test` builds the binary, runs `go test ./...`, starts a local server, and opens `/?test=true` in headless Chrome. The `?test=true` mode injects a local Jasmine runner and executes [public/tests/site.spec.js](/vfast/data/code/statuspage/public/tests/site.spec.js).

## Stripe

Create a recurring Stripe Price for `$19/month`, then set:

- `STRIPE_SECRET_KEY`
- `STRIPE_PRICE_ID`
- `STRIPE_WEBHOOK_SECRET`
- `APP_URL`

The checkout endpoint is `POST /checkout/create`. Stripe webhooks should point at `POST /stripe/webhook`.
