# Bot Telegram Monitoring Server

Bot Telegram ringan yang ditulis menggunakan **Go (Golang)** untuk memantau metrik resource server host secara real-time. Bot ini berjalan di dalam **Docker container** dan didesain memiliki penggunaan memori (RAM) yang sangat minimal (~6MB - 12MB RAM saja) menggunakan Docker *multi-stage build*.

---

## 🚀 Fitur Utama

1. **Sangat Ringan**: Dibuat menggunakan Go dan dijalankan di atas image Alpine yang minimalis, menghemat penggunaan memori server Anda.
2. **Real-time Monitoring**: Dapatkan status server terbaru kapan saja melalui perintah Telegram.
3. **Alert Kritis Sistem**: Pengiriman notifikasi peringatan jika penggunaan CPU, RAM, atau Disk melewati batas (*threshold*) yang ditentukan. Dilengkapi fitur *hysteresis* dan *cooldown* untuk mencegah spamming.
4. **Laporan Harian Otomatis**: Bot akan mengirimkan ringkasan status resource harian secara otomatis pada jam yang Anda tentukan, lengkap dengan statistik penggunaan puncak (*peak usage*) selama 24 jam terakhir.
5. **Keamanan Ketat**: Bot dikonfigurasi hanya merespons perintah dari ID Chat pemilik server yang terdaftar pada konfigurasi.

---

## 🛠️ Panduan Instalasi & Deploy

### 1. Persiapan Akun & Token Telegram
1. Buat bot baru dengan mengirimkan pesan `/newbot` ke [@BotFather](https://t.me/BotFather) di Telegram.
2. Simpan **Token HTTP API** yang diberikan.
3. Dapatkan **Chat ID** Telegram Anda dengan mengirimkan pesan apa saja ke [@userinfobot](https://t.me/userinfobot) di Telegram. Catat ID angka yang diberikan.

### 2. Konfigurasi Environment Variables
Salin template konfigurasi `.env.example` ke `.env`:

```bash
cp .env.example .env
```

Buka file `.env` dan lengkapi konfigurasi berikut (pastikan file `.env` **tidak di-commit** ke Git demi keamanan):

```env
# Telegram Bot Configuration
TELEGRAM_BOT_TOKEN=YOUR_TELEGRAM_BOT_TOKEN_HERE
TELEGRAM_CHAT_ID=YOUR_TELEGRAM_CHAT_ID_HERE

# Threshold Peringatan (dalam persen)
CPU_THRESHOLD=85
RAM_THRESHOLD=85
DISK_THRESHOLD=90

# Pengaturan Monitoring
POLL_INTERVAL=10          # Jeda waktu cek resource (detik)
ALERT_COOLDOWN=30         # Waktu jeda antarsent-alert (menit)
DAILY_REPORT_TIME=08:00   # Jam pengiriman laporan harian otomatis (HH:MM)

# Zona Waktu Server
TZ=Asia/Jakarta
```

### 3. Menjalankan Menggunakan Docker Compose
Pastikan Docker dan Docker Compose sudah terinstal di server Anda. Jalankan perintah berikut untuk membuild dan menjalankan bot di background:

```bash
docker compose up -d --build
```

### 4. Memantau Status Container
* Untuk melihat log aktivitas bot:
  ```bash
  docker compose logs -f
  ```
* Untuk melihat penggunaan memori/RAM container bot:
  ```bash
  docker stats monitor-server-bot
  ```

---

## 📱 Daftar Perintah Bot Telegram

Kirim perintah berikut langsung di chat pribadi dengan bot Anda:
* `/status` - Menampilkan status CPU, RAM, Disk, dan Uptime server saat ini.
* `/ping` - Memeriksa koneksi dan memastikan bot merespons.
* `/help` - Menampilkan bantuan dan daftar perintah yang tersedia.
