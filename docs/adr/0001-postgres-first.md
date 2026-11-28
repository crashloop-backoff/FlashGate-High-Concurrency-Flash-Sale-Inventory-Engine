# ADR-0001: Implementasi PostgreSQL-Only sebagai Baseline Konkurensi

- **Status:** Accepted
- **Tanggal:** 2026-11-24

## Konteks
FlashGate harus membuktikan 0% overselling di bawah beban tinggi sekaligus menunjukkan dampak performa nyata dari penambahan Redis di Fase 4. Jika Redis langsung dipasang sejak awal, kita tidak memiliki angka pembanding (*baseline*) dan menyembunyikan masalah konsistensi di lapisan *persistence*.

## Keputusan
Pada Fase 3, seluruh logika pemotongan stok, *idempotency*, dan pengembalian stok (*cancel*/*sweeper*) dieksekusi 100% secara atomik di **PostgreSQL 16** via `pgx/v5`:
1. Pemotongan stok memakai *conditional atomic update* (`UPDATE events SET available_stock = available_stock - $1 WHERE id = $2 AND available_stock >= $1 ...`) di dalam satu transaksi SQL bersama `INSERT INTO orders`.
2. *Idempotency* dijamin lewat `UNIQUE (idempotency_key)` di level database, bukan pengecekan *in-memory*.
3. *Background sweeper* memakai `SELECT ... FOR UPDATE SKIP LOCKED` agar beberapa *worker* atau *race* dengan endpoint `/pay` tidak pernah mengembalikan stok dua kali.

## Konsekuensi
- **Positif:** *Strong consistency* (ACID) terjamin sejak hari pertama; *invariant* stok sulit ditembus.
- **Negatif (Disengaja untuk Diukur):** Seluruh *request* berebut *row-level lock* pada satu baris `events` yang sama. Pada konkurensi tinggi, *lock contention* dan antrean `pgxpool` akan menaikkan *p95/p99 latency*—inilah *bottleneck* yang akan kita ukur di Batch 3B dan kita pecahkan di Fase 4.
