# Benchmark Plan — FlashGate (Ticket War)

Dokumen ini mengunci parameter pengujian beban k6 agar perbandingan **Fase 3 (Postgres-only)** dan **Fase 4 (Redis Lua + Postgres)** dilakukan secara adil (*apples-to-apples*).

## 1. Spesifikasi Lingkungan
- **Mesin Uji:** Lenovo IdeaPad Slim 3 (Intel Core i5-12450H, 16 GB RAM, Fedora Linux).
- **Topologi:** Docker Compose lokal (`api`, `postgres`, `prometheus`, `grafana`; ditambah `redis` pada Fase 4).
- **Pool DB:** `DB_MAX_CONNS=25`, `DB_MIN_CONNS=5`.

## 2. Skenario Uji Utama (`tests/load/ticket-war.js`)
- **Stok Awal Event:** `total_stock = 1000`, `available_stock = 1000`.
- **Pola Beban (Ramping VUs):**
  - `0s - 5s`: *Warm-up* ke 20 VU.
  - `5s - 35s`: *Spike* ke 200 VU (semua menembak `POST /v1/orders` dengan `quantity = 1` dan `Idempotency-Key` unik per percobaan, plus 10% simulasi *retry* dengan `Idempotency-Key` duplikat).
  - `35s - 45s`: *Ramp-down* ke 0 VU.
- **Simulasi Siklus Order:** Sebagian VU yang sukses mendapat `RESERVED` menembak `POST /v1/orders/{id}/pay` (60%) atau `POST /v1/orders/{id}/cancel` (10%), sisanya dibiarkan `RESERVED` untuk menguji *sweeper*.

## 3. Metrik Wajib yang Dicatat
1. **Throughput:** Requests per Second (RPS) rata-rata dan puncak.
2. **Latency:** `p50`, `p95`, dan `p99` untuk `POST /v1/orders`.
3. **Reliability:** Persentase HTTP `5xx` (target 0% atau dicatat jika *pool exhausted*) vs kegagalan bisnis normal (`409 Conflict` / `OUT_OF_STOCK`).
4. **DB Saturation:** `flashgate_db_pool_acquired_conns` dan `flashgate_db_pool_wait_count_total` dari Grafana.
5. **Correctness:** Hasil eksekusi query audit di `docs/benchmark/correctness.md`.
