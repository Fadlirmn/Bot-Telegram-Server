# Rencana Implementasi: Bot Telegram Monitoring Server Ringan (Docker + Go)

Membuat bot Telegram untuk memonitor status server (CPU, RAM, Disk, Uptime) dengan footprint RAM yang sangat kecil (< 15MB) menggunakan bahasa pemrograman **Go (Golang)** dan dideploy menggunakan **Docker (Multi-stage build)**.

## Fitur Utama
1. **Sangat Ringan**: Menggunakan Go dengan Docker multi-stage build (`alpine`), menghasilkan image berukuran kecil (~20MB) dan penggunaan RAM yang minimal (~10MB).
2. **Alert Sistem Kritis (Real-time)**: Bot akan mendeteksi jika penggunaan CPU, RAM, atau Disk melebihi ambang batas (threshold) yang ditentukan dan mengirim pesan peringatan. Dilengkapi sistem *cooldown* agar tidak spamming.
3. **Laporan Harian Otomatis**: Bot akan mengirimkan ringkasan penggunaan resource server secara otomatis setiap hari pada jam tertentu.
4. **Command Telegram**:
   - `/status` - Melihat penggunaan CPU, RAM, Disk, dan Uptime saat ini.
   - `/help` - Menampilkan daftar perintah yang tersedia.
   - `/ping` - Memeriksa apakah bot aktif dan merespons.

## Parameter Konfigurasi (Environment Variables)
Semua konfigurasi sensitif disimpan dalam environment variables (mengikuti aturan keamanan):
- `TELEGRAM_BOT_TOKEN`: Token API bot Telegram.
- `TELEGRAM_CHAT_ID`: ID Chat Telegram tujuan pengiriman alert dan laporan.
- `POLL_INTERVAL`: Interval pengecekan resource dalam detik (default: `10`).
- `CPU_THRESHOLD`: Batas kritis penggunaan CPU (%) (default: `85`).
- `RAM_THRESHOLD`: Batas kritis penggunaan RAM (%) (default: `85`).
- `DISK_THRESHOLD`: Batas kritis penggunaan Disk (%) (default: `90`).
- `ALERT_COOLDOWN`: Durasi cooldown peringatan (menit) untuk mencegah spam (default: `30`).
- `DAILY_REPORT_TIME`: Waktu pengiriman laporan harian dalam format `HH:MM` (default: `08:00`).

---

## Rencana Perubahan Struktur File

### [NEW] [main.go](file:///home/sumbul/Dokumen/monitor-server/main.go)
Berisi logika utama bot Telegram, inisialisasi client Telegram, scheduler laporan harian, polling system stats, command handler, dan logger.

### [NEW] [go.mod](file:///home/sumbul/Dokumen/monitor-server/go.mod)
File modul Go untuk mendefinisikan dependency proyek. Kita akan menggunakan:
- `github.com/shirou/gopsutil/v3` untuk membaca resource sistem.
- `github.com/go-telegram-bot-api/telegram-bot-api/v5` untuk interaksi Telegram API.
- `github.com/joho/godotenv` untuk membaca file `.env` secara lokal.

### [NEW] [Dockerfile](file:///home/sumbul/Dokumen/monitor-server/Dockerfile)
Dockerfile multi-stage untuk membuild binary Go pada stage pertama dan menjalankannya di stage kedua menggunakan image `alpine` yang sangat minimalis dan aman.

### [NEW] [docker-compose.yml](file:///home/sumbul/Dokumen/monitor-server/docker-compose.yml)
File docker-compose untuk menjalankan bot dengan mounting volume yang diperlukan (misal read-only `/proc`, `/sys`, dan root disk `/hostfs` jika diperlukan untuk monitoring akurat host dari dalam container).

### [NEW] [.env.example](file:///home/sumbul/Dokumen/monitor-server/.env.example)
Template konfigurasi environment variable tanpa kredensial asli untuk keamanan sesuai protokol.

---

## Pengecekan Sumber Daya di Dalam Container (Host Monitoring)
Karena bot akan berjalan di dalam Docker container, untuk membaca statistik CPU, RAM, dan Disk dari **Host OS** yang sebenarnya (bukan container), kita perlu memetakan mount volume host berikut ke dalam container secara read-only:
- `/proc` host -> `/host/proc` (opsional, gopsutil mendukung env `HOST_PROC=/host/proc`)
- `/sys` host -> `/host/sys` (opsional, gopsutil mendukung env `HOST_SYS=/host/sys`)
- `/` host -> `/host` (untuk memantau penggunaan disk host, gopsutil mendukung env `HOST_ETC=/host/etc` dan custom path disk).

---

## Rencana Verifikasi

### Pengujian Otomatis / Manual
1. **Build Docker Image**:
   Memastikan build multi-stage berhasil tanpa error.
2. **Pengecekan RAM Footprint**:
   Menjalankan container dan memeriksa penggunaan RAM menggunakan perintah `docker stats`. RAM harus di bawah 20MB.
3. **Pengecekan Fitur Command**:
   Mengirimkan perintah `/status` dan `/ping` ke bot untuk memastikan respons yang cepat dan data CPU/RAM/Disk terbaca dengan benar.
4. **Pengecekan Alert Kritis**:
   Menurunkan threshold konfigurasi (misal RAM_THRESHOLD=10) untuk memicu pengiriman alert kritis ke Telegram.
5. **Pengecekan Laporan Harian**:
   Mengubah waktu `DAILY_REPORT_TIME` ke beberapa menit di depan untuk memverifikasi laporan otomatis terkirim tepat waktu.
