# Transjakarta Fleet Management — Backend

Layanan backend Go untuk manajemen armada: menerima lokasi kendaraan lewat MQTT, menyimpannya ke PostgreSQL, menyajikannya lewat REST API (Gin), dan mengirim event `geofence_entry` ke RabbitMQ saat kendaraan masuk radius 50 m dari titik tertentu. Semua komponen berjalan dengan satu perintah `docker compose up`.

## Arsitektur

Diagram interaktif: [docs/architecture.html](docs/architecture.html) (buka di browser).

```
 publisher (mock, tiap 2 dtk)
      │ MQTT QoS 1  /fleet/vehicle/{id}/location
      ▼
 ┌──────────┐      ┌──────────────────────────── app ───────────────────────────┐
 │mosquitto │ ───▶ │ subscriber → validasi → ambil lokasi terakhir → INSERT       │
 └──────────┘      │                                   │                          │
                   │                    luar → dalam radius?  ──ya──▶ publish     │
                   │ REST API Gin :8080 ◀── query ─────┤                    │      │
                   └───────────────────────────────────┼────────────────────┼──────┘
                                                       ▼                    ▼
                                                 ┌──────────┐   ┌──────────────────────┐
                                                 │ postgres │   │ rabbitmq             │
                                                 │ vehicle_ │   │ fleet.events (topic) │
                                                 │ locations│   │  └─geofence.#─▶      │
                                                 └──────────┘   │    geofence_alerts   │
                                                                └──────────┬───────────┘
                                                                           ▼
                                                                  worker (log alert)
```

Satu modul Go, tiga binary dalam satu image Docker:

| Binary | Tugas |
|---|---|
| `app` | subscriber MQTT, simpan ke PostgreSQL, deteksi geofence + publish ke RabbitMQ, REST API |
| `worker` | konsumen queue `geofence_alerts`, mencatat setiap alert ke log |
| `publisher` | mock kendaraan: kirim lokasi 2 kendaraan tiap 2 detik |

Keputusan arsitektur (tiga binary satu image, state geofence dari DB) dirangkum pada bagian Asumsi & Keputusan di bawah.

## Prasyarat

- Docker + Docker Compose v2
- Go 1.27 (opsional, hanya untuk menjalankan uji di luar Docker)

## Menjalankan

```bash
docker compose up -d --build
docker compose ps
```

Dalam ±60 detik `postgres`, `rabbitmq`, `mosquitto`, `app` berstatus `healthy`; `worker` dan `publisher` `running`.

Port 5432 di host sudah dipakai? Ganti port host PostgreSQL:

```bash
POSTGRES_HOST_PORT=5434 docker compose up -d --build
```

Hentikan dan hapus data:

```bash
docker compose down -v
```

## Layanan & port

| Layanan | Port host | Keterangan |
|---|---|---|
| app (REST API) | 8080 | `GET /vehicles/{id}/location`, `GET /vehicles/{id}/history`, `GET /health` |
| postgres | 5432 (`POSTGRES_HOST_PORT`) | user/pass/db `fleet` |
| rabbitmq | 5672, 15672 | UI manajemen `http://localhost:15672` (fleet / fleet) |
| mosquitto | 1883 | MQTT tanpa auth |

## REST API

| Endpoint | Sukses | Error |
|---|---|---|
| `GET /vehicles/{vehicle_id}/location` | 200 `{"vehicle_id","latitude","longitude","timestamp"}` (timestamp terbesar) | 404 `{"error":"location not found"}`, 500 `{"error":"internal error"}` |
| `GET /vehicles/{vehicle_id}/history?start=&end=` | 200 array urut `timestamp` naik, `start ≤ timestamp ≤ end`; kosong → `[]` | 400 (start/end hilang, bukan integer, `start > end`), 500 |
| `GET /health` | 200 `{"status":"ok"}` | 503 `{"status":"unhealthy"}` |

## Cara menguji

### 1. Data masuk ke PostgreSQL

```bash
docker compose exec postgres psql -U fleet -d fleet -c "select count(*) from vehicle_locations"
```

Ulangi setelah 10 detik — bertambah ±10 baris (2 kendaraan × 5 pesan). Tiap pesan valid yang tersimpan juga tercatat di log:

```bash
docker compose logs app | grep "location saved" | tail -3
```

### 2. REST API (curl)

```bash
curl -s localhost:8080/vehicles/B1234XYZ/location
curl -si localhost:8080/vehicles/UNKNOWN/location
curl -s "localhost:8080/vehicles/B1234XYZ/history?start=$(( $(date +%s)-60 ))&end=$(date +%s)"
curl -si "localhost:8080/vehicles/B1234XYZ/history?start=abc&end=1"
```

### 3. Postman

Import `postman/fleet-management.postman_collection.json`, jalankan Collection Runner. Variabel `start`/`end` otomatis diisi 1 jam terakhir bila kosong. Lewat CLI:

```bash
npx --yes newman run postman/fleet-management.postman_collection.json
```

### 4. Event geofence

```bash
docker compose logs -f worker | grep "geofence alert"
```

`B1234XYZ` melintasi geofence bolak-balik (satu lintasan ±48 dtk) → satu alert per masuk. `B5678ABC` mengorbit 1 km dari titik pusat → tidak pernah memicu alert. Di RabbitMQ UI (`http://localhost:15672`, fleet/fleet) terlihat exchange `fleet.events` ter-bind ke queue `geofence_alerts` dengan key `geofence.#`.

Contoh pesan di queue:

```json
{"vehicle_id":"B1234XYZ","event":"geofence_entry","location":{"latitude":-6.1754,"longitude":106.8267},"timestamp":1715003456}
```

### 5. Payload tidak valid

Hentikan publisher dulu agar jumlah baris mudah dibandingkan:

```bash
docker compose stop publisher
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B1234XYZ/location -m '{'
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B1234XYZ/location -m '{"vehicle_id":"B1234XYZ","latitude":-6.2,"longitude":106.8}'
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B1234XYZ/location -m '{"vehicle_id":"B1234XYZ","latitude":91,"longitude":106.8,"timestamp":1715003456}'
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B1234XYZ/location -m '{"vehicle_id":"OTHER","latitude":-6.2,"longitude":106.8,"timestamp":1715003456}'
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B1234XYZ/location -m '{"vehicle_id":"B1234XYZ","latitude":-6.2,"longitude":106.8,"timestamp":1715003456,"speed":40}'
docker compose logs app | grep "invalid location dropped"
docker compose start publisher
```

Kelima pesan dibuang dengan log `WARN`, tidak tersimpan, app tetap berjalan.

### 6. Uji unit & integrasi

```bash
go vet ./...
go test ./...
```

Uji store dilewati tanpa database. Untuk menjalankannya terhadap PostgreSQL compose:

```bash
TEST_DATABASE_URL="postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable" go test ./internal/store/ -v
```

### 7. Graceful shutdown

```bash
docker compose stop app
docker compose logs app | tail -2
```

Baris terakhir `shutdown complete`.

## Konfigurasi (env)

Template konfigurasi tersedia di `.env.example`. Salin ke `.env` jika ingin mengubah nilai default:

```bash
cp .env.example .env
```

| Variabel | Default | Dipakai oleh |
|---|---|---|
| `MQTT_BROKER_URL` | `tcp://mosquitto:1883` | app, publisher |
| `DATABASE_URL` | `postgres://fleet:fleet@postgres:5432/fleet?sslmode=disable` | app |
| `RABBITMQ_URL` | `amqp://fleet:fleet@rabbitmq:5672/` | app, worker |
| `HTTP_ADDR` | `:8080` | app |
| `GEOFENCE_LAT` | `-6.1754` | app, publisher |
| `GEOFENCE_LON` | `106.8272` | app, publisher |
| `GEOFENCE_RADIUS_M` | `50` | app, publisher |
| `PUBLISH_INTERVAL` | `2s` | publisher |
| `POSTGRES_HOST_PORT` | `5432` | docker compose (port host PostgreSQL) |

Nilai tidak valid (bukan angka, radius/interval ≤ 0) membuat proses berhenti dengan error yang menyebut nama variabelnya.

## Asumsi & keputusan

- **Nama tabel `vehicle_locations`.** Brief menulis `vehicle_loctions`; dianggap typo dan dikoreksi. Kolom sesuai brief (`vehicle_id`, `latitude`, `longitude`, `timestamp`) ditambah `id BIGSERIAL`. Tabel dan index `(vehicle_id, timestamp DESC)` dibuat otomatis saat app start.
- **Titik geofence.** Brief tidak menyebut titik; default Monas (`-6.1754, 106.8272`), dapat diganti lewat env. Jarak dihitung haversine (R = 6 371 000 m); jarak tepat 50 m dihitung di dalam.
- **Definisi entry.** Event dikirim hanya saat transisi luar → dalam (lokasi sebelumnya dari DB di luar radius, atau belum ada). Kendaraan yang tetap di dalam tidak memicu event berulang; keluar lalu masuk lagi memicu event baru.
- **Pesan terlambat / duplikat.** MQTT QoS 1 bisa mengirim ulang. Pesan duplikat tetap tersimpan, tapi tidak memicu event kedua karena lokasi terakhir sudah di dalam. Pesan dengan timestamp lebih lama dari lokasi terakhir tersimpan tanpa event.
- **Validasi payload.** Field wajib lengkap, tanpa field tambahan, angka bertipe number (bukan string), timestamp integer > 0, lat ±90, lon ±180, `vehicle_id` 1–32 karakter dan sama dengan id di topik, payload ≤ 1 KB. Pesan invalid dibuang dengan log `WARN`.
- **Kegagalan RabbitMQ.** Lokasi tetap tersimpan; event yang gagal dipublish dicatat `ERROR` (tanpa antrean ulang). Publisher memakai publisher confirm dan menyambung ulang otomatis pada pesan berikutnya.
- **History** selalu array JSON (`[]` bila kosong); `start`/`end` wajib, integer Unix detik.
- **Satu instance app.** Handler MQTT memproses pesan berurutan sehingga throughput dibatasi latensi satu query DB per pesan; cukup untuk demo. Skala besar → partisi per `vehicle_id` atau worker pool, dan shared subscription bila app lebih dari satu.
- **Di luar cakupan:** auth/TLS MQTT & API, banyak geofence, event `geofence_exit`, retensi data, metrics.

## Struktur folder

```
cmd/
  app/          subscriber MQTT + REST API + publisher event geofence
  worker/       konsumen geofence_alerts
  publisher/    mock lokasi kendaraan (rute melintasi & mengorbit geofence)
internal/
  api/          router Gin
  config/       pembacaan env
  geofence/     haversine + deteksi transisi masuk
  ingest/       handler pesan MQTT
  location/     model + validasi payload
  mq/           topologi RabbitMQ, event, publisher
  retry/        retry koneksi saat start
  store/        PostgreSQL (pgx)
docs/           diagram arsitektur
mosquitto/      konfigurasi broker
postman/        Postman Collection
```
