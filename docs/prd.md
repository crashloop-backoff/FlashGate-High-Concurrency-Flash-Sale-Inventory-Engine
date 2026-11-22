# PRD — FlashGate (High-Concurrency Flash-Sale & Inventory Engine)

## 1. Tujuan Proyek
FlashGate adalah layanan backend dan studi kasus rekayasa DevOps untuk menangani beban puncak *flash-sale* (ticket war). Proyek ini **bukan** aplikasi e-commerce komersial, melainkan sistem eksperimen terukur untuk membuktikan:
1. Jaminan **0% overselling** di bawah ratusan hingga ribuan *request* konkuren.
2. Perbandingan performa empiris (*before-after*) antara **PostgreSQL-only** vs **Redis Atomic Reservation + PostgreSQL Reconciliation** menggunakan k6, Prometheus, dan Grafana.
3. Siklus operasional otomatis (CI/CD, *image hardening*, *observability*, dan *alerting*).

## 2. Skenario Inti (Ticket War)
1. Admin membuat satu *event* dengan stok terbatas (contoh *benchmark*: `total_stock = 1.000`, `available_stock = 1.000`).
2. Saat jendela *flash-sale* dibuka (`starts_at <= now < ends_at`), ratusan Virtual Users (VU) menembak endpoint reservasi secara bersamaan.
3. Setiap permintaan reservasi wajib membawa `Idempotency-Key` untuk mencegah pemotongan stok ganda akibat *retry* jaringan.
4. Reservasi yang berhasil mengunci stok dalam status `RESERVED` selama jendela waktu terbatas (`TTL`, default 5 menit; dapat dikonfigurasi lewat *environment variable* untuk kebutuhan pengujian).
5. Pengguna dapat menyelesaikan pembayaran simulasi (`PAID`) atau membatalkan (`CANCELLED`) sebelum waktu habis.
6. Jika tidak dibayar hingga `expires_at`, *background sweeper* otomatis mengubah status menjadi `EXPIRED` dan mengembalikan stok ke `available_stock`.

## 3. Ruang Lingkup (In-Scope MVP)
- Manajemen *event* minimalis (buat *event* & cek sisa stok).
- Reservasi tiket atomik dengan *idempotency key* (maksimal 5 tiket per transaksi).
- Simulasi konfirmasi pembayaran (`PAID`) dan pembatalan (`CANCELLED`).
- *Background sweeper* untuk mengembalikan stok dari reservasi `EXPIRED`.
- Eksposur endpoint `/health` dan `/metrics` (Prometheus).

## 4. Batas Tegas (Out-of-Scope)
Fitur berikut **sengaja tidak dibangun** agar fokus tetap pada konkurensi dan DevOps:
- Autentikasi/otorisasi penuh (JWT/OAuth) — identitas pengguna disimulasikan lewat header `X-User-ID` (asumsi: proyek ini arena *benchmark* lokal, bukan API publik).
- Integrasi *payment gateway* pihak ketiga, keranjang belanja (*multi-item cart*), dan alur *refund*.
- Antarmuka pengguna (Frontend UI).
- Orkestrasi Kubernetes atau *deployment multi-region*.

## 5. Kriteria Sukses Terukur
- **Correctness (Hard Invariant):** Setelah *load test* selesai, persamaan berikut wajib bernilai benar dengan selisih 0:
  `available_stock + SUM(orders.quantity WHERE status IN ('RESERVED', 'PAID')) == total_stock`
- **Idempotency:** Pengiriman ulang *request* dengan `Idempotency-Key` yang sama tidak pernah mengurangi stok dua kali dan selalu mengembalikan hasil *order* yang sama.
- **Observability:** Metrik RED (*Rate, Errors, Duration*), *DB connection pool*, dan *Redis hit/miss* tampil di Grafana; *alert* menyala dalam waktu < 60 detik saat Postgres atau Redis dimatikan paksa.
- **Evidence-Based Performance:** Tersedia data perbandingan RPS, p95/p99 *latency*, dan *error rate* antara fase Postgres-only dan fase Redis pada spesifikasi mesin yang sama.
