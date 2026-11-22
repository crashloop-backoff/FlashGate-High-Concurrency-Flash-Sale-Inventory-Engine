# Observability & Metrics Catalog

Semua metrik diekspos pada endpoint `GET /metrics` (format Prometheus) dengan *scrape interval* **5 detik** agar lonjakan *spike* saat *load test* k6 terekam detail.

## 1. Katalog Metrik

### A. HTTP RED Metrics (Wajib sejak Batch 2B)
| Nama Metrik | Tipe | Label | Kegunaan |
| :--- | :--- | :--- | :--- |
| `flashgate_http_requests_total` | Counter | `method`, `route`, `status_code` | Mengukur **Rate** (RPS) dan **Errors** (4xx/5xx). Gunakan template rute (`/v1/events/{event_id}`), bukan UUID mentah, untuk mencegah *cardinality explosion*. |
| `flashgate_http_request_duration_seconds` | Histogram | `method`, `route` | Mengukur **Duration** (p50, p95, p99 latency). Bucket: `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5]`. |
| `flashgate_http_inflight_requests` | Gauge | — | Jumlah *request* aktif yang sedang diproses secara bersamaan. |

### B. Domain & Concurrency Metrics (Aktif mulai Fase 3)
| Nama Metrik | Tipe | Label | Kegunaan |
| :--- | :--- | :--- | :--- |
| `flashgate_reservations_total` | Counter | `result` (`success`, `out_of_stock`, `idempotent_replay`, `error`) | Memisahkan kegagalan bisnis normal (`out_of_stock`) dari kegagalan sistem (`error`). |
| `flashgate_sweeper_expired_total` | Counter | — | Jumlah *order* `RESERVED` yang berhasil dikembalikan stoknya oleh *background sweeper*. |

### C. Database & Cache Pool Metrics
| Nama Metrik | Tipe | Label | Kegunaan |
| :--- | :--- | :--- | :--- |
| `flashgate_db_pool_acquired_conns` | Gauge | — | Koneksi `pgxpool` yang sedang dipakai (mendeteksi *connection pool exhaustion*). |
| `flashgate_db_pool_idle_conns` | Gauge | — | Koneksi `pgxpool` yang menganggur. |
| `flashgate_db_pool_wait_count_total` | Counter | — | Total permintaan yang terpaksa antre menunggu koneksi DB bebas (indikator *lock contention*). |
| `flashgate_redis_ops_total` | Counter | `op`, `result` (`hit`, `miss`, `error`) | Rasio keberhasilan operasi Redis (aktif mulai Fase 4). |

## 2. Panel Utama Dashboard Grafana
1. **Request Rate (RPS) per Route:**
   `sum by (route) (rate(flashgate_http_requests_total[15s]))`
2. **Latency p95 & p99 (ms):**
   `histogram_quantile(0.95, sum by (le, route) (rate(flashgate_http_request_duration_seconds_bucket[30s]))) * 1000`
3. **5xx Error Rate (%):**
   `100 * sum(rate(flashgate_http_requests_total{status_code=~"5.."}[30s])) / clamp_min(sum(rate(flashgate_http_requests_total[30s])), 1)`
4. **Reservation Breakdown (Success vs Out-of-Stock vs Error):**
   `sum by (result) (rate(flashgate_reservations_total[15s]))`
5. **DB Connection Pool Saturation:**
   `flashgate_db_pool_acquired_conns` vs `flashgate_db_pool_idle_conns` + `rate(flashgate_db_pool_wait_count_total[30s])`
