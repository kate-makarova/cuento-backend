package Services

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	healthLogDir    = "logs/health"
	healthInterval  = 30 * time.Second
	healthRetention = 30 * 24 * time.Hour
	healthCSVHeader = "timestamp,ram_total,ram_used,ram_available,cpu_pct\n"
)

var prevCPUSample *cpuSample

type HealthUpdate struct {
	RAM map[string]HealthRAMEntry `json:"ram"`
	CPU map[string]HealthCPUEntry `json:"cpu"`
}

var (
	healthSubscribers   = map[int]bool{}
	healthSubscribersMu sync.Mutex
	healthBroadcaster   func(HealthUpdate)
)

func SetHealthBroadcaster(fn func(HealthUpdate)) {
	healthBroadcaster = fn
}

func SubscribeHealth(userID int) {
	healthSubscribersMu.Lock()
	healthSubscribers[userID] = true
	healthSubscribersMu.Unlock()
}

func UnsubscribeHealth(userID int) {
	healthSubscribersMu.Lock()
	delete(healthSubscribers, userID)
	healthSubscribersMu.Unlock()
}

func GetHealthSubscribers() []int {
	healthSubscribersMu.Lock()
	defer healthSubscribersMu.Unlock()
	ids := make([]int, 0, len(healthSubscribers))
	for id := range healthSubscribers {
		ids = append(ids, id)
	}
	return ids
}

func InitHealthMonitor() {
	if err := os.MkdirAll(healthLogDir, 0755); err != nil {
		log.Printf("Health monitor: failed to create log directory: %v", err)
		return
	}

	go func() {
		ticker := time.NewTicker(healthInterval)
		defer ticker.Stop()

		// Run immediately on start, then on every tick
		runHealthCheck()
		for range ticker.C {
			runHealthCheck()
		}
	}()
}

type systemMemStats struct {
	Total     uint64
	Available uint64
	Used      uint64
}

func readSystemMem() (systemMemStats, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return systemMemStats{}, err
	}
	defer f.Close()

	fields := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		fields[key] = val * 1024 // /proc/meminfo values are in kB
	}

	total := fields["MemTotal"]
	available := fields["MemAvailable"]
	return systemMemStats{
		Total:     total,
		Available: available,
		Used:      total - available,
	}, nil
}

type cpuSample struct {
	total uint64
	idle  uint64
}

func readCPUSample() (*cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		// cpu user nice system idle iowait irq softirq steal guest guest_nice
		fields := strings.Fields(line)[1:] // drop "cpu" label
		var vals [10]uint64
		for i := 0; i < len(fields) && i < 10; i++ {
			vals[i], _ = strconv.ParseUint(fields[i], 10, 64)
		}
		idle := vals[3] + vals[4] // idle + iowait
		total := vals[0] + vals[1] + vals[2] + vals[3] + vals[4] +
			vals[5] + vals[6] + vals[7] + vals[8] + vals[9]
		return &cpuSample{total: total, idle: idle}, nil
	}
	return nil, fmt.Errorf("cpu line not found in /proc/stat")
}

func runHealthCheck() {
	now := time.Now()

	mem, memErr := readSystemMem()
	cur, cpuErr := readCPUSample()

	// Build CSV row: timestamp,ram_total,ram_used,ram_available,cpu_pct
	var ramTotal, ramUsed, ramAvailable string
	if memErr == nil {
		ramTotal = strconv.FormatUint(mem.Total, 10)
		ramUsed = strconv.FormatUint(mem.Used, 10)
		ramAvailable = strconv.FormatUint(mem.Available, 10)
	}

	var cpuPct string
	if cpuErr == nil && prevCPUSample != nil {
		deltaTotal := cur.total - prevCPUSample.total
		deltaIdle := cur.idle - prevCPUSample.idle
		if deltaTotal > 0 {
			cpuPct = fmt.Sprintf("%.2f", float64(deltaTotal-deltaIdle)/float64(deltaTotal)*100)
		}
	}
	if cur != nil {
		prevCPUSample = cur
	}

	row := fmt.Sprintf("%s,%s,%s,%s,%s\n",
		now.Format("2006-01-02 15:04:05"),
		ramTotal, ramUsed, ramAvailable,
		cpuPct,
	)

	csvPath := filepath.Join(healthLogDir, fmt.Sprintf("health_%s.csv", now.Format("2006-01-02")))
	isNew := false
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(csvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Health monitor: failed to open CSV file: %v", err)
		return
	}
	defer f.Close()

	if isNew {
		if _, err := f.WriteString(healthCSVHeader); err != nil {
			log.Printf("Health monitor: failed to write CSV header: %v", err)
			return
		}
	}
	if _, err := f.WriteString(row); err != nil {
		log.Printf("Health monitor: failed to write CSV row: %v", err)
	}

	if healthBroadcaster != nil {
		ts := now.Format("2006-01-02 15:04:05")
		update := HealthUpdate{
			RAM: map[string]HealthRAMEntry{},
			CPU: map[string]HealthCPUEntry{},
		}
		if memErr == nil {
			update.RAM[ts] = HealthRAMEntry{Total: mem.Total, Used: mem.Used, Available: mem.Available}
		}
		if cpuPct != "" {
			if pct, err := strconv.ParseFloat(cpuPct, 64); err == nil {
				update.CPU[ts] = HealthCPUEntry{Pct: pct}
			}
		}
		go healthBroadcaster(update)
	}

	pruneOldHealthLogs()
}

type HealthRAMEntry struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

type HealthCPUEntry struct {
	Pct float64 `json:"pct"`
}

type HealthData struct {
	RAM map[string]HealthRAMEntry `json:"ram"`
	CPU map[string]HealthCPUEntry `json:"cpu"`
}

// ReadHealthData reads CSV files covering [start, end] and returns parsed entries.
func ReadHealthData(start, end time.Time) HealthData {
	result := HealthData{
		RAM: map[string]HealthRAMEntry{},
		CPU: map[string]HealthCPUEntry{},
	}

	// Iterate day by day across the requested range
	for day := start.Truncate(24 * time.Hour); !day.After(end.Truncate(24 * time.Hour)); day = day.Add(24 * time.Hour) {
		csvPath := filepath.Join(healthLogDir, fmt.Sprintf("health_%s.csv", day.Format("2006-01-02")))
		f, err := os.Open(csvPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		scanner.Scan() // skip header
		for scanner.Scan() {
			fields := strings.Split(scanner.Text(), ",")
			if len(fields) < 5 {
				continue
			}
			ts, err := time.ParseInLocation("2006-01-02 15:04:05", fields[0], time.Local)
			if err != nil || ts.Before(start) || ts.After(end) {
				continue
			}
			key := fields[0]

			ramTotal, e1 := strconv.ParseUint(fields[1], 10, 64)
			ramUsed, e2 := strconv.ParseUint(fields[2], 10, 64)
			ramAvailable, e3 := strconv.ParseUint(fields[3], 10, 64)
			if e1 == nil && e2 == nil && e3 == nil {
				result.RAM[key] = HealthRAMEntry{Total: ramTotal, Used: ramUsed, Available: ramAvailable}
			}

			if fields[4] != "" {
				if pct, err := strconv.ParseFloat(fields[4], 64); err == nil {
					result.CPU[key] = HealthCPUEntry{Pct: pct}
				}
			}
		}
		f.Close()
	}

	return result
}

func pruneOldHealthLogs() {
	cutoff := time.Now().Add(-healthRetention)
	entries, err := os.ReadDir(healthLogDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(healthLogDir, entry.Name()))
		}
	}
}
