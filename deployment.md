# Deployment

Production deploys run on the server as `administrator`.

The live app is served by systemd on the prod machine and proxied by nginx:

```text
Cloudflare DNS A statuspage.app.nz -> 93.127.141.100
nginx :80 server_name statuspage.app.nz -> 127.0.0.1:8096
systemd statuspage.service -> /nvme0n1-disk/code/statuspage/statuspage
```

The static bucket build is also published to Cloudflare R2 under `appstatic/statuspage` for CDN/static checks.

## Server

SSH alias:

```sh
alias sscp='ssh -o StrictHostKeyChecking=no administrator@93.127.141.100'
```

Repository path:

```sh
/nvme0n1-disk/code/statuspage
```

First-time checkout:

```sh
sscp
cd /nvme0n1-disk/code
git clone https://github.com/lee101/statuspage.git statuspage
cd statuspage
```

Subsequent deploys:

```sh
sscp
cd /nvme0n1-disk/code/statuspage
git pull --ff-only
./deploy.sh
```

## What `deploy.sh` does

1. Runs `bun run build` to rebuild the Go-embedded production output and server binary.
2. Runs `go test ./...`.
3. Builds a bucket-specific static copy with `PUBLIC_BASE_PATH=/statuspage` into `dist/appstatic`.
4. Syncs `dist/appstatic` to `s3://appstatic/statuspage` using the Cloudflare R2 endpoint.

After deploying on the prod machine, restart the service:

```sh
echo ilu | sudo -S systemctl restart statuspage.service
```

Verify nginx and systemd:

```sh
systemctl status statuspage.service
curl -fsS http://127.0.0.1:8096/health
curl -I -H 'Host: statuspage.app.nz' http://127.0.0.1/
```

The public static URL is:

```text
https://appstatic.app.nz/statuspage/
```

Defaults can be overridden:

```sh
R2_BUCKET=appstatic R2_PREFIX=statuspage ./deploy.sh
```

The script can use existing `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, or `CLOUDFLARE_R2_ACCESS_KEY_ID` and `CLOUDFLARE_R2_SECRET_ACCESS_KEY` from `.env`.

## DNS

`statuspage.app.nz` is a proxied Cloudflare A record pointing at:

```text
93.127.141.100
```
