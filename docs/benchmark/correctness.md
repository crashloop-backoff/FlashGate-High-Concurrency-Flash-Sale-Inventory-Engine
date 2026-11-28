# Correctness & Zero-Overselling Audit Protocol

Setelah setiap eksekusi *load test* selesai (dan *sweeper* diberi jeda 15 detik untuk menyelesaikan *order* yang kedaluwarsa), jalankan seluruh query audit berikut terhadap PostgreSQL.

## 1. Hard Invariant (Zero Overselling)
Selisih (`discrepancy`) WAJIB bernilai **0**.

```sql
SELECT
    e.id,
    e.total_stock,
    e.available_stock,
    COALESCE(SUM(o.quantity) FILTER (WHERE o.status IN ('RESERVED', 'PAID')), 0) AS committed_qty,
    (e.available_stock + COALESCE(SUM(o.quantity) FILTER (WHERE o.status IN ('RESERVED', 'PAID')), 0)) - e.total_stock AS discrepancy
FROM events e
LEFT JOIN orders o ON o.event_id = e.id
GROUP BY e.id, e.total_stock, e.available_stock;
```

## 2. Cek Distribusi Status & Duplikasi Idempotency
Pastikan tidak ada `available_stock` negatif dan tidak ada `idempotency_key` ganda.

```sql
SELECT status, COUNT(*) AS order_count, COALESCE(SUM(quantity), 0) AS total_qty
FROM orders
GROUP BY status
ORDER BY status;

SELECT idempotency_key, COUNT(*)
FROM orders
GROUP BY idempotency_key
HAVING COUNT(*) > 1;
```

## Kriteria Lulus
- `discrepancy = 0` pada semua *event*.
- Query duplikasi `idempotency_key` mengembalikan **0 rows**.
- Tidak ada baris `events` dengan `available_stock` kurang dari 0 atau melebihi `total_stock`.
