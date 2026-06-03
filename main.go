package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type SystemStats struct {
	CPUUsage    float64
	RAMTotal    uint64
	RAMUsed     uint64
	RAMPercent  float64
	DiskTotal   uint64
	DiskUsed    uint64
	DiskPercent float64
	Uptime      uint64
}

var (
	statsMutex   sync.RWMutex
	currentStats SystemStats
	peakCPU      float64
	peakRAM      float64
	peakDisk     float64
)

func main() {
	// Load environmental variables from .env if present (useful for local development)
	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found or unable to load, using system environment variables")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("Fatal: TELEGRAM_BOT_TOKEN is not set")
	}

	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	var chatID int64
	if chatIDStr != "" {
		var err error
		chatID, err = strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			log.Fatalf("Fatal: Invalid TELEGRAM_CHAT_ID: %v", err)
		}
	} else {
		log.Println("Info: TELEGRAM_CHAT_ID is not set. Proactive alerts and daily reports are disabled.")
	}

	// Configuration variables
	pollInterval := getEnvInt("POLL_INTERVAL", 10)
	cpuThreshold := getEnvInt("CPU_THRESHOLD", 85)
	ramThreshold := getEnvInt("RAM_THRESHOLD", 85)
	diskThreshold := getEnvInt("DISK_THRESHOLD", 90)
	alertCooldown := getEnvInt("ALERT_COOLDOWN", 30) // in minutes
	dailyReportTime := getEnv("DAILY_REPORT_TIME", "08:00")

	log.Printf("Starting monitoring bot...")
	log.Printf("Settings: PollInterval=%ds, CPUThreshold=%d%%, RAMThreshold=%d%%, DiskThreshold=%d%%, Cooldown=%dm, DailyReportTime=%s",
		pollInterval, cpuThreshold, ramThreshold, diskThreshold, alertCooldown, dailyReportTime)

	// Initialize Telegram Bot
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Fatal: Error initializing Telegram Bot: %v", err)
	}
	log.Printf("Authorized on account @%s", bot.Self.UserName)

	// Register bot command menu
	setBotCommands(bot)

	// Start stats gathering loop (runs first system poll synchronously to populate initial cache)
	updateStats()
	go startStatsGathering(bot, chatID, pollInterval, cpuThreshold, ramThreshold, diskThreshold, alertCooldown)

	// Start daily report scheduler
	go startDailyReportScheduler(bot, chatID, dailyReportTime)

	// Send startup notification
	sendStartupMessage(bot, chatID)

	// Listen for Telegram updates/commands
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.IsCommand() {
				handleCommand(bot, update.Message)
			}
		} else if update.CallbackQuery != nil {
			handleCallbackQuery(bot, update.CallbackQuery)
		}
	}
}

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	command := strings.ToLower(msg.Command())
	username := ""
	if msg.From != nil {
		username = msg.From.UserName
	}
	log.Printf("Received command: /%s from Chat ID: %d, Username: %s", command, msg.Chat.ID, username)

	switch command {
	case "status":
		handleStatusCommand(bot, msg.Chat.ID)
	case "vps", "vps_status", "vpsstatus":
		handleVPSCommand(bot, msg.Chat.ID)
	case "docker", "docker_status", "dockerstatus":
		handleDockerStatusCommand(bot, msg.Chat.ID)
	case "dockerlogs", "logs", "docker_logs":
		handleDockerLogsCommand(bot, msg.Chat.ID, msg.CommandArguments())
	case "ping":
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🏓 Pong! Bot monitoring aktif dan berjalan lancar.")
		bot.Send(reply)
	case "help":
		helpText := "📌 *Daftar Perintah Bot Monitoring:*\n\n" +
			"👉 `/status` - Menampilkan status CPU, RAM, Disk, dan Uptime server saat ini.\n" +
			"👉 `/vps` - Menampilkan detail info dan spesifikasi VPS.\n" +
			"👉 `/docker` - Menampilkan status container Docker saat ini.\n" +
			"👉 `/dockerlogs` - Menampilkan log container Docker (menggunakan tombol aktif).\n" +
			"👉 `/ping` - Memeriksa konektivitas dan status keaktifan bot.\n" +
			"👉 `/help` - Menampilkan bantuan ini."
		reply := tgbotapi.NewMessage(msg.Chat.ID, helpText)
		reply.ParseMode = "Markdown"
		bot.Send(reply)
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Perintah tidak dikenal. Ketik `/help` untuk daftar perintah.")
		bot.Send(reply)
	}
}

func setBotCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{
			Command:     "status",
			Description: "Melihat status CPU, RAM, Disk & Uptime server",
		},
		{
			Command:     "vps",
			Description: "Melihat detail info dan spesifikasi VPS",
		},
		{
			Command:     "docker",
			Description: "Melihat status container Docker",
		},
		{
			Command:     "dockerlogs",
			Description: "Melihat log container Docker",
		},
		{
			Command:     "ping",
			Description: "Memeriksa konektivitas ke bot",
		},
		{
			Command:     "help",
			Description: "Menampilkan daftar perintah bantuan",
		},
	}

	config := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(config); err != nil {
		log.Printf("Error setting bot commands: %v", err)
	} else {
		log.Println("Bot commands menu registered successfully!")
	}
}

func handleCallbackQuery(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	callback := tgbotapi.NewCallback(query.ID, "")
	if _, err := bot.Request(callback); err != nil {
		log.Printf("Error acknowledging callback: %v", err)
	}

	data := query.Data
	if strings.HasPrefix(data, "dockerlogs:") {
		containerName := strings.TrimPrefix(data, "dockerlogs:")
		handleDockerLogsCommand(bot, query.Message.Chat.ID, containerName)
	}
}

func getLoadAverage() string {
	procPath := "/proc/loadavg"
	if envProc := os.Getenv("HOST_PROC"); envProc != "" {
		procPath = envProc + "/loadavg"
	}
	data, err := os.ReadFile(procPath)
	if err != nil {
		return "N/A"
	}
	parts := strings.Fields(string(data))
	if len(parts) >= 3 {
		return fmt.Sprintf("%s, %s, %s", parts[0], parts[1], parts[2])
	}
	return "N/A"
}

func handleVPSCommand(bot *tgbotapi.BotAPI, chatID int64) {
	hostInfo, err := host.Info()
	if err != nil {
		log.Printf("Error fetching host info: %v", err)
	}

	var cpuModel string
	var cpuCores int32
	var cpuMhz float64
	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		cpuModel = cpuInfos[0].ModelName
		cpuCores = int32(len(cpuInfos))
		cpuMhz = cpuInfos[0].Mhz
	} else {
		cpuModel = "N/A"
	}

	virtualMem, err := mem.VirtualMemory()
	var ramInfo string
	if err == nil {
		ramUsedGB := float64(virtualMem.Used) / (1024 * 1024 * 1024)
		ramTotalGB := float64(virtualMem.Total) / (1024 * 1024 * 1024)
		ramInfo = fmt.Sprintf("%.2f GB / %.2f GB (%.1f%%)", ramUsedGB, ramTotalGB, virtualMem.UsedPercent)
	} else {
		ramInfo = "N/A"
	}

	swapMem, err := mem.SwapMemory()
	var swapInfo string
	if err == nil {
		swapUsedGB := float64(swapMem.Used) / (1024 * 1024 * 1024)
		swapTotalGB := float64(swapMem.Total) / (1024 * 1024 * 1024)
		swapInfo = fmt.Sprintf("%.2f GB / %.2f GB (%.1f%%)", swapUsedGB, swapTotalGB, swapMem.UsedPercent)
	} else {
		swapInfo = "N/A"
	}

	loadAvg := getLoadAverage()

	var hostname, platform, kernel, arch, virt string
	if hostInfo != nil {
		hostname = hostInfo.Hostname
		platform = fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion)
		kernel = hostInfo.KernelVersion
		arch = hostInfo.KernelArch
		if hostInfo.VirtualizationSystem != "" {
			virt = fmt.Sprintf("%s (%s)", hostInfo.VirtualizationSystem, hostInfo.VirtualizationRole)
		} else {
			virt = "Baremetal"
		}
	} else {
		hostname, platform, kernel, arch, virt = "N/A", "N/A", "N/A", "N/A", "N/A"
	}

	text := fmt.Sprintf(
		"⚙️ *INFORMASI SISTEM VPS*\n"+
			"-----------------------------\n"+
			"🏷️ *Hostname:* `%s`\n"+
			"🐧 *OS/Platform:* `%s`\n"+
			"📦 *Kernel:* `%s (%s)`\n"+
			"🖥️ *Virtualization:* `%s`\n"+
			"⚙️ *Processor:* `%s (%d Cores @ %.0f MHz)`\n"+
			"📊 *Load Average:* `%s`\n"+
			"📟 *Memory (RAM):* `%s`\n"+
			"💾 *Swap Memory:* `%s`\n"+
			"-----------------------------",
		hostname, platform, kernel, arch, virt, cpuModel, cpuCores, cpuMhz, loadAvg, ramInfo, swapInfo,
	)

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Error sending VPS response: %v", err)
	}
}

func handleDockerStatusCommand(bot *tgbotapi.BotAPI, chatID int64) {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}\t{{.Status}}\t{{.State}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		reply := tgbotapi.NewMessage(chatID, "❌ *Gagal mendapatkan status Docker.*\nPastikan Docker daemon berjalan dan service memiliki hak akses socket.\n\nDetail error:\n`"+err.Error()+"`")
		reply.ParseMode = "Markdown"
		bot.Send(reply)
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		reply := tgbotapi.NewMessage(chatID, "🐳 *Tidak ada container Docker yang berjalan atau terinstal.*")
		reply.ParseMode = "Markdown"
		bot.Send(reply)
		return
	}

	var builder strings.Builder
	builder.WriteString("🐳 *STATUS DOCKER CONTAINER*\n")
	builder.WriteString("-----------------------------\n")

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		status := parts[1]
		state := parts[2]

		var statusEmoji string
		switch state {
		case "running":
			statusEmoji = "🟢"
		case "paused":
			statusEmoji = "🟡"
		case "exited":
			statusEmoji = "🔴"
		case "restarting":
			statusEmoji = "🔄"
		default:
			statusEmoji = "⚪"
		}

		builder.WriteString(fmt.Sprintf("%s `%s`\n   ↳ Status: _%s_\n", statusEmoji, name, status))
	}
	builder.WriteString("-----------------------------")

	reply := tgbotapi.NewMessage(chatID, builder.String())
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Error sending Docker status response: %v", err)
	}
}

func handleDockerLogsCommand(bot *tgbotapi.BotAPI, chatID int64, args string) {
	containerName := strings.TrimSpace(args)
	if containerName == "" {
		cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}\t{{.State}}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			reply := tgbotapi.NewMessage(chatID, "❌ *Gagal mengambil daftar container.*\nPastikan Docker daemon berjalan.\n\nDetail: `"+err.Error()+"`")
			reply.ParseMode = "Markdown"
			bot.Send(reply)
			return
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			name := parts[0]
			state := parts[1]

			var statusEmoji string
			switch state {
			case "running":
				statusEmoji = "🟢"
			case "paused":
				statusEmoji = "🟡"
			case "exited":
				statusEmoji = "🔴"
			case "restarting":
				statusEmoji = "🔄"
			default:
				statusEmoji = "⚪"
			}

			buttonText := fmt.Sprintf("%s %s", statusEmoji, name)
			buttonData := fmt.Sprintf("dockerlogs:%s", name)
			btn := tgbotapi.NewInlineKeyboardButtonData(buttonText, buttonData)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
		}

		if len(rows) == 0 {
			reply := tgbotapi.NewMessage(chatID, "🐳 *Tidak ada container Docker yang terinstal.*")
			reply.ParseMode = "Markdown"
			bot.Send(reply)
			return
		}

		keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
		reply := tgbotapi.NewMessage(chatID, "📋 *PILIH CONTAINER DOCKER UNTUK CEK LOG:*")
		reply.ParseMode = "Markdown"
		reply.ReplyMarkup = keyboard
		bot.Send(reply)
		return
	}

	cmd := exec.Command("docker", "logs", "--tail", "30", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ *Gagal mengambil log untuk container `%s`.*\nDetail error: `%s`", containerName, err.Error()))
		reply.ParseMode = "Markdown"
		bot.Send(reply)
		return
	}

	logContent := strings.TrimSpace(string(output))
	if logContent == "" {
		logContent = "(Log kosong / tidak ada output)"
	}

	if len(logContent) > 3800 {
		logContent = logContent[len(logContent)-3800:]
		logContent = "... [truncated]\n" + logContent
	}

	text := fmt.Sprintf("📄 *LOG DOCKER: %s (30 Baris Terakhir)*\n"+
		"-----------------------------\n"+
		"```\n%s\n```\n"+
		"-----------------------------", containerName, logContent)

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Error sending Docker logs response: %v", err)
	}
}

func handleStatusCommand(bot *tgbotapi.BotAPI, chatID int64) {
	statsMutex.RLock()
	stats := currentStats
	statsMutex.RUnlock()

	ramUsedGB := float64(stats.RAMUsed) / (1024 * 1024 * 1024)
	ramTotalGB := float64(stats.RAMTotal) / (1024 * 1024 * 1024)
	diskUsedGB := float64(stats.DiskUsed) / (1024 * 1024 * 1024)
	diskTotalGB := float64(stats.DiskTotal) / (1024 * 1024 * 1024)

	days := stats.Uptime / (24 * 3600)
	hours := (stats.Uptime % (24 * 3600)) / 3600
	minutes := (stats.Uptime % 3600) / 60

	text := fmt.Sprintf(
		"🖥️ *STATUS SERVER SAAT INI*\n"+
			"-----------------------------\n"+
			"⏱️ *Uptime:* %d hari, %d jam, %d menit\n"+
			"🔥 *CPU Usage:* `%.1f%%`\n"+
			"📟 *RAM Usage:* `%.1f%%` (%.2f GB / %.2f GB)\n"+
			"💾 *Disk Usage:* `%.1f%%` (%.2f GB / %.2f GB)\n"+
			"-----------------------------",
		days, hours, minutes,
		stats.CPUUsage,
		stats.RAMPercent, ramUsedGB, ramTotalGB,
		stats.DiskPercent, diskUsedGB, diskTotalGB,
	)

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = "Markdown"
	if _, err := bot.Send(reply); err != nil {
		log.Printf("Error sending status response: %v", err)
	}
}

func updateStats() {
	var stats SystemStats

	// 1. Get Uptime
	hostInfo, err := host.Info()
	if err == nil {
		stats.Uptime = hostInfo.Uptime
	} else {
		log.Printf("Error fetching uptime: %v", err)
	}

	// 2. Get CPU Usage
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercents) > 0 {
		stats.CPUUsage = cpuPercents[0]
	} else {
		log.Printf("Error fetching CPU usage: %v", err)
	}

	// 3. Get RAM Usage
	virtualMem, err := mem.VirtualMemory()
	if err == nil {
		stats.RAMTotal = virtualMem.Total
		stats.RAMUsed = virtualMem.Used
		stats.RAMPercent = virtualMem.UsedPercent
	} else {
		log.Printf("Error fetching RAM usage: %v", err)
	}

	// 4. Get Disk Usage
	// Check if host filesystem is mounted to /hostfs in Docker
	diskPath := "/"
	if _, err := os.Stat("/hostfs"); err == nil {
		diskPath = "/hostfs"
	}
	diskUsage, err := disk.Usage(diskPath)
	if err == nil {
		stats.DiskTotal = diskUsage.Total
		stats.DiskUsed = diskUsage.Used
		stats.DiskPercent = diskUsage.UsedPercent
	} else {
		log.Printf("Error fetching Disk usage: %v", err)
	}

	// Save to global variables safely
	statsMutex.Lock()
	currentStats = stats
	if stats.CPUUsage > peakCPU {
		peakCPU = stats.CPUUsage
	}
	if stats.RAMPercent > peakRAM {
		peakRAM = stats.RAMPercent
	}
	if stats.DiskPercent > peakDisk {
		peakDisk = stats.DiskPercent
	}
	statsMutex.Unlock()
}

func startStatsGathering(bot *tgbotapi.BotAPI, chatID int64, pollInterval int, cpuThreshold, ramThreshold, diskThreshold, alertCooldown int) {
	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()

	alertActive := make(map[string]bool)
	lastAlertTime := make(map[string]time.Time)
	cooldownDuration := time.Duration(alertCooldown) * time.Minute

	for range ticker.C {
		updateStats()

		if chatID == 0 {
			continue
		}

		statsMutex.RLock()
		stats := currentStats
		statsMutex.RUnlock()

		// Monitor CPU
		cpuT := float64(cpuThreshold)
		if stats.CPUUsage > cpuT {
			if !alertActive["cpu"] || time.Since(lastAlertTime["cpu"]) > cooldownDuration {
				sendAlert(bot, chatID, "CPU", stats.CPUUsage, cpuT)
				alertActive["cpu"] = true
				lastAlertTime["cpu"] = time.Now()
			}
		} else if stats.CPUUsage <= cpuT-5 { // 5% hysteresis
			if alertActive["cpu"] {
				sendRecovery(bot, chatID, "CPU", stats.CPUUsage, cpuT)
				alertActive["cpu"] = false
				lastAlertTime["cpu"] = time.Time{}
			}
		}

		// Monitor RAM
		ramT := float64(ramThreshold)
		if stats.RAMPercent > ramT {
			if !alertActive["ram"] || time.Since(lastAlertTime["ram"]) > cooldownDuration {
				sendAlert(bot, chatID, "RAM", stats.RAMPercent, ramT)
				alertActive["ram"] = true
				lastAlertTime["ram"] = time.Now()
			}
		} else if stats.RAMPercent <= ramT-5 { // 5% hysteresis
			if alertActive["ram"] {
				sendRecovery(bot, chatID, "RAM", stats.RAMPercent, ramT)
				alertActive["ram"] = false
				lastAlertTime["ram"] = time.Time{}
			}
		}

		// Monitor Disk
		diskT := float64(diskThreshold)
		if stats.DiskPercent > diskT {
			if !alertActive["disk"] || time.Since(lastAlertTime["disk"]) > cooldownDuration {
				sendAlert(bot, chatID, "Disk", stats.DiskPercent, diskT)
				alertActive["disk"] = true
				lastAlertTime["disk"] = time.Now()
			}
		} else if stats.DiskPercent <= diskT-3 { // 3% hysteresis
			if alertActive["disk"] {
				sendRecovery(bot, chatID, "Disk", stats.DiskPercent, diskT)
				alertActive["disk"] = false
				lastAlertTime["disk"] = time.Time{}
			}
		}
	}
}

func sendAlert(bot *tgbotapi.BotAPI, chatID int64, metric string, val float64, threshold float64) {
	text := fmt.Sprintf("⚠️ *ALERT CRITICAL: %s Usage High!*\n-----------------------------\n📊 Penggunaan saat ini: `%.1f%%` (Threshold: `%.1f%%`)", metric, val, threshold)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending alert: %v", err)
	}
}

func sendRecovery(bot *tgbotapi.BotAPI, chatID int64, metric string, val float64, threshold float64) {
	text := fmt.Sprintf("✅ *RECOVERY: %s Kembali Normal*\n-----------------------------\n📊 Penggunaan saat ini: `%.1f%%` (Threshold: `%.1f%%`)", metric, val, threshold)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending recovery message: %v", err)
	}
}

func startDailyReportScheduler(bot *tgbotapi.BotAPI, chatID int64, dailyReportTime string) {
	if chatID == 0 {
		log.Println("Info: TELEGRAM_CHAT_ID is not set. Daily report scheduler is disabled.")
		return
	}
	lastSentDate := ""

	// Ticker run checks every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		currentHHMM := now.Format("15:04")
		currentDate := now.Format("2006-01-02")

		if currentHHMM == dailyReportTime && currentDate != lastSentDate {
			sendDailyReport(bot, chatID)
			lastSentDate = currentDate
		}
	}
}

func sendDailyReport(bot *tgbotapi.BotAPI, chatID int64) {
	statsMutex.RLock()
	stats := currentStats
	pCPU := peakCPU
	pRAM := peakRAM
	pDisk := peakDisk
	statsMutex.RUnlock()

	ramUsedGB := float64(stats.RAMUsed) / (1024 * 1024 * 1024)
	ramTotalGB := float64(stats.RAMTotal) / (1024 * 1024 * 1024)
	diskUsedGB := float64(stats.DiskUsed) / (1024 * 1024 * 1024)
	diskTotalGB := float64(stats.DiskTotal) / (1024 * 1024 * 1024)

	days := stats.Uptime / (24 * 3600)
	hours := (stats.Uptime % (24 * 3600)) / 3600
	minutes := (stats.Uptime % 3600) / 60

	text := fmt.Sprintf(
		"📅 *LAPORAN MONITORING HARIAN*\n"+
			"-----------------------------\n"+
			"⏱️ *Uptime:* %d hari, %d jam, %d menit\n\n"+
			"📊 *Status Saat Ini:*\n"+
			"🔥 CPU: `%.1f%%`\n"+
			"📟 RAM: `%.1f%%` (%.2f GB / %.2f GB)\n"+
			"💾 Disk: `%.1f%%` (%.2f GB / %.2f GB)\n\n"+
			"📈 *Penggunaan Puncak (24 Jam Terakhir):*\n"+
			"🔥 Peak CPU: `%.1f%%`\n"+
			"📟 Peak RAM: `%.1f%%`\n"+
			"💾 Peak Disk: `%.1f%%`\n"+
			"-----------------------------",
		days, hours, minutes,
		stats.CPUUsage,
		stats.RAMPercent, ramUsedGB, ramTotalGB,
		stats.DiskPercent, diskUsedGB, diskTotalGB,
		pCPU, pRAM, pDisk,
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending daily report: %v", err)
	} else {
		// Reset peak records for the next 24 hour cycle
		statsMutex.Lock()
		peakCPU = 0
		peakRAM = 0
		peakDisk = 0
		statsMutex.Unlock()
		log.Println("Daily report sent. Peak statistics reset.")
	}
}

func sendStartupMessage(bot *tgbotapi.BotAPI, chatID int64) {
	if chatID == 0 {
		return
	}
	text := "🚀 *Bot Monitoring Server Aktif!*\n\n" +
		"Bot sekarang memonitor server secara real-time.\n" +
		"Ketik `/status` untuk melihat metrik terbaru, atau `/help` untuk daftar perintah."
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending startup message: %v", err)
	}
}

// Helpers for env parameters parsing
func getEnv(key, defaultValue string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Printf("Warning: invalid integer value for %s: %s. Using default: %d", key, valStr, defaultValue)
		return defaultValue
	}
	return val
}
