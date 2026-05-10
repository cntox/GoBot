package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/showwin/speedtest-go/speedtest"
)

func main() {
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:   26593951,
		AppHash: "98d8a63d74b4082b129defe4f941ab4d",
	})

	if err != nil {
		log.Fatal(err)
	}

	client.Conn()

	// Login using your Bot Token
	client.LoginBot("8793661673:AAEn5NK7sJ-dV328XBEVgeWhoDWbAICLzUI")

	client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		// m.Text is a function, so we call it using ()
		text := m.Text()

		switch {
		case strings.HasPrefix(text, "/ping"):
			return handlePing(m)
		case strings.HasPrefix(text, "/speedtest"):
			return handleSpeedtest(m)
		case strings.HasPrefix(text, "/stats"):
			return handleStats(m)
		}

		return nil
	}, telegram.IsCommand, telegram.IsGroup)

	client.Idle()
}

// --- Handlers ---

func handlePing(m *telegram.NewMessage) error {
	start := time.Now()
	_, err := m.Reply("🏓 <i>Pinging...</i>",{ParseMode: "html"})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)
	res := fmt.Sprintf("🏓 <b>Pong!</b>\n\n<b>Latency:</b> <code>%v</code>\n<b>Server Time:</b> <code>%s</code>", 
		elapsed, time.Now().Format("15:04:05 MST"))

	_, err = m.Reply(res, {ParseMode: "html"})
	return err
}

func handleSpeedtest(m *telegram.NewMessage) error {
	_, _ = m.Reply("🚀 <i>Starting speedtest... Please wait.</i>", &telegram.ReplyOptions{ParseMode: "html"})

	st := speedtest.New()
	serverList, err := st.FetchServers()
	if err != nil {
		_, err = m.Reply("❌ <b>Error fetching servers.</b>", &telegram.ReplyOptions{ParseMode: "html"})
		return err
	}

	targets, err := serverList.FindServer([]int{})
	if err != nil || len(targets) == 0 {
		_, err = m.Reply("❌ <b>No suitable server found.</b>", &telegram.ReplyOptions{ParseMode: "html"})
		return err
	}

	s := targets[0]
	s.PingTest(nil)
	s.DownloadTest()
	s.UploadTest()

	res := fmt.Sprintf("🚀 <b>Speedtest Results</b>\n\n<b>Server:</b> <code>%s</code>\n<b>Ping:</b> <code>%v</code>\n<b>Download:</b> <code>%.2f Mbps</code>\n<b>Upload:</b> <code>%.2f Mbps</code>",
		s.Host, s.Latency, s.DLSpeed, s.ULSpeed)

	_, err = m.Reply(res, {ParseMode: "html"})
	return err
}

func handleStats(m *telegram.NewMessage) error {
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Percent(0, false)
	d, _ := disk.Usage("/")

	var cpuUsed float64
	if len(c) > 0 {
		cpuUsed = c[0]
	}

	const gb = 1024 * 1024 * 1024

	res := fmt.Sprintf(
		"📊 <b>System Statistics</b>\n\n"+
		"<b>CPU Usage:</b> <code>%.2f%%</code>\n\n"+
		"<b>RAM Memory:</b>\n"+
		"• Total: <code>%.2f GB</code>\n"+
		"• Used: <code>%.2f GB</code> (<code>%.1f%%</code>)\n"+
		"• Free: <code>%.2f GB</code>\n\n"+
		"<b>Disk Storage:</b>\n"+
		"• Total: <code>%.2f GB</code>\n"+
		"• Used: <code>%.2f GB</code> (<code>%.1f%%</code>)\n"+
		"• Free: <code>%.2f GB</code>",
		cpuUsed,
		float64(v.Total)/gb, float64(v.Used)/gb, v.UsedPercent, float64(v.Free)/gb,
		float64(d.Total)/gb, float64(d.Used)/gb, d.UsedPercent, float64(d.Free)/gb,
	)

	_, err := m.Reply(res, &telegram.ReplyOptions{ParseMode: "html"})
	return err
}
