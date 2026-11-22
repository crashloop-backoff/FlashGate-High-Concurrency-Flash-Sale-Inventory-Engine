# CI/CD Pipeline Specification

Pipeline berjalan di **GitHub Actions** (`.github/workflows/ci.yml`) dan dirancang bertahap: gerbang kualitas dasar aktif sejak **Batch 2B**, lalu *security scan* dan *release* ke GHCR diaktifkan di **Batch 5A**.

## 1. Pemicu (Triggers)
- **Pull Request ke `main`:** Menjalankan tahap CI (*Lint*, *Test*, *Docker Build check*). Tidak melakukan *push* ke registry.
- **Push ke `main` / Git Tag (`v*.*.*`):** Menjalankan seluruh tahap CI + *Vulnerability Scan* + *Publish* image ke GitHub Container Registry (`ghcr.io/<username>/flashgate`).

## 2. Tahapan Pipeline & Syarat Lulus (Gates)

| Tahap | Perintah / Tool | Aktif Sejak | Syarat Lulus |
| :--- | :--- | :--- | :--- |
| **1. Lint & Vet** | `golangci-lint run` & `go vet ./...` | Batch 2B | Nol error lint atau potensi bug statis. |
| **2. Test & Race Check** | `go test -race -cover ./...` | Batch 2B | 100% test lulus tanpa *data race*. |
| **3. Docker Build** | `docker build` (*multi-stage*) | Batch 2B | Image ter-build bersih tanpa error. |
| **4. Image Scan** | Trivy (`aquasecurity/trivy-action`) | Batch 5A | Nol kerentanan tingkat `CRITICAL` pada OS/binary. |
| **5. Publish GHCR** | `docker/build-push-action` | Batch 5A | Tag `sha-<short>` (tiap merge ke `main`) dan `vX.Y.Z` (tiap release tag). |

## 3. Aturan Keamanan Pipeline
- Gunakan `GITHUB_TOKEN` bawaan dengan prinsip *least privilege* (`contents: read`, `packages: write` hanya di job *publish*).
- Pin versi GitHub Action minimal pada *major tag* stabil (misal `actions/checkout@v4`).
- Dilarang menyisipkan secret produksi atau file `.env` ke dalam layer Docker image.
