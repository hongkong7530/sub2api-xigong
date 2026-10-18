# Xigong · Sub2API Custom Edition

**An independent community edition of [Sub2API](https://github.com/Wei-Shaw/sub2api), with an immersive interface and operational extensions.** This is not an official upstream release.

[Live demo](https://xigong.cloud) · [中文文档](README.md) · [Screenshots](docs/SCREENSHOTS.md) · [License](LICENSE)

![Actual live demo homepage](docs/screenshots/home-desktop.png)

## Highlights

- Immersive wallpaper-based homepage, liquid-glass surfaces, particle branding, responsive layouts and motion controls.
- Editable site content, documentation, partner showcase and desktop/mobile image focal points.
- Channel monitoring combining observed requests with active probes, historical status, first-token latency and output throughput.
- Model plaza, group-specific model pricing and allowlists, long-context thresholds and service-tier policies.
- API key folders, group categories, per-key prompt templates and usage/cost visibility.
- Optional role-based cluster with mTLS, admission control, idempotent settlement and local recovery evidence.

Upstream account pools, authentication, compatible gateway protocols, billing, payment integrations and administration remain part of the source. No public administrator credentials are provided. The demo is a live service: do not load-test it or upload confidential information.

## Quick start

Build this edition from source. Upstream container images do **not** contain these modifications.

```bash
git clone https://github.com/hongkong7530/sub2api-xigong.git
cd sub2api-xigong
cp deploy/.env.example deploy/.env
# Set strong independent passwords and persistent JWT/TOTP secrets in deploy/.env.
# Prefer BIND_HOST=127.0.0.1 behind an HTTPS reverse proxy.
docker compose --env-file deploy/.env -f deploy/docker-compose.xigong.yml up -d --build
```

Open `http://127.0.0.1:8080`, locally or through an SSH tunnel. Read the [deployment guide](docs/DEPLOYMENT.md) before exposing a server or upgrading an existing database. The detailed guides are currently primarily in Chinese.

## Development

Use Go **1.27.2**, Node.js **20+** and pnpm **9**. Install frontend dependencies with the frozen lockfile; run the frontend build before `go build -tags embed`. The frontend outputs directly into `backend/internal/web/dist`.

See [development](docs/DEVELOPMENT.md), [architecture](docs/ARCHITECTURE.md), [contributing](CONTRIBUTING.md), [security](SECURITY.md) and [known limitations](docs/KNOWN_LIMITATIONS.md). Do not replace this customized binary using the upstream in-app updater.

## Provenance and license

This repository is a sanitized source snapshot, not a copy of private deployment history. It excludes production configuration, credentials, private keys, databases and operational records. The live deployment may differ from this source snapshot.

Code retains upstream **LGPL-3.0** terms and attribution; see [LICENSE](LICENSE), [COPYING](COPYING) and [NOTICE.md](NOTICE.md). Fonts and artwork have separate terms; see [asset notices](docs/ASSETS.md). The maintainer supplied the wallpapers and confirmed free-commercial-use permission; individual redistribution terms have not been independently established here. Their presence is not a grant of LGPL rights to third-party artwork.

Respect provider terms, local law and privacy obligations. There is no warranty, performance guarantee or entitlement to third-party services.
