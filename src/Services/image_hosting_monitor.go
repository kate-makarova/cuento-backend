package Services

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	imageHostingPort     = 8888
	imageHostingInterval = 5 * time.Second
	imageHostingDefaultRamLimitBytes = int64(5 * 1024 * 1024)
)

func StartImageHostingMonitor(db *sql.DB) {
	go func() {
		portClosed := false

		ticker := time.NewTicker(imageHostingInterval)
		defer ticker.Stop()

		for range ticker.C {
			enabled, _ := GetGlobalSetting("image_hosting_enabled", db)
			if enabled != "y" {
				if portClosed {
					if err := setPortAccess(imageHostingPort, true); err != nil {
						log.Printf("image hosting monitor: failed to reopen port on disable: %v", err)
					} else {
						portClosed = false
					}
				}
				continue
			}

			threshold := GetImageHostingRamLimitBytes(db)

			total, err := totalImageHostingRSS()
			if err != nil {
				log.Printf("image hosting monitor: failed to read RSS: %v", err)
				continue
			}

			if total > threshold && !portClosed {
				if err := setPortAccess(imageHostingPort, false); err != nil {
					log.Printf("image hosting monitor: failed to close port %d: %v", imageHostingPort, err)
				} else {
					log.Printf("image hosting monitor: RSS %d bytes exceeds threshold %d, closed port %d", total, threshold, imageHostingPort)
					portClosed = true
				}
			} else if total <= threshold && portClosed {
				if err := setPortAccess(imageHostingPort, true); err != nil {
					log.Printf("image hosting monitor: failed to reopen port %d: %v", imageHostingPort, err)
				} else {
					log.Printf("image hosting monitor: RSS %d bytes below threshold %d, reopened port %d", total, threshold, imageHostingPort)
					portClosed = false
				}
			}
		}
	}()
}

func GetImageHostingRamLimitBytes(db *sql.DB) int64 {
	val, err := GetGlobalSetting("image_hosting_ram_limit_mb", db)
	return getRamLimitBytesFromValue(val, err != nil)
}

func getRamLimitBytesFromValue(val string, dbErr bool) int64 {
	if dbErr {
		return imageHostingDefaultRamLimitBytes
	}
	mb, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil || mb <= 0 {
		return imageHostingDefaultRamLimitBytes
	}
	return mb * 1024 * 1024
}

// EnableImageHosting opens port 8888 and starts the nginx-image-hosting systemd service.
func EnableImageHosting() error {
	if err := setPortAccess(imageHostingPort, true); err != nil {
		return fmt.Errorf("open port: %w", err)
	}
	if err := startImageHostingNginx(); err != nil {
		_ = setPortAccess(imageHostingPort, false)
		return fmt.Errorf("start nginx: %w", err)
	}
	return nil
}

// DisableImageHosting stops the nginx-image-hosting systemd service and closes port 8888.
func DisableImageHosting() error {
	_ = stopImageHostingNginx()
	if err := setPortAccess(imageHostingPort, false); err != nil {
		return fmt.Errorf("close port: %w", err)
	}
	return nil
}

// IsNginxImageHostingActive reports whether the nginx-image-hosting systemd service is active.
func IsNginxImageHostingActive() bool {
	out, err := exec.Command("systemctl", "is-active", "nginx-image-hosting").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

func startImageHostingNginx() error {
	out, err := exec.Command("systemctl", "start", "nginx-image-hosting").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func stopImageHostingNginx() error {
	out, err := exec.Command("systemctl", "stop", "nginx-image-hosting").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// totalImageHostingRSS returns total resident memory (bytes) of all nginx processes plus this process.
func totalImageHostingRSS() (int64, error) {
	total := int64(0)

	nginxPIDs, err := getNginxPIDs()
	if err != nil {
		return 0, fmt.Errorf("listing nginx PIDs: %w", err)
	}
	for _, pid := range nginxPIDs {
		rss, err := getProcessRSSBytes(pid)
		if err != nil {
			// Process may have exited between listing and reading; skip silently.
			continue
		}
		total += rss
	}

	selfRSS, err := getProcessRSSBytes(os.Getpid())
	if err != nil {
		return 0, fmt.Errorf("reading self RSS: %w", err)
	}
	total += selfRSS

	return total, nil
}

func getNginxPIDs() ([]int, error) {
	out, err := exec.Command("pgrep", "nginx").Output()
	if err != nil {
		// pgrep exits 1 when no processes are found — not an error for us.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	var pids []int
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// getProcessRSSBytes reads VmRSS from /proc/<pid>/status and returns it in bytes.
func getProcessRSSBytes(pid int) (int64, error) {
	return readRSSBytesFromPath(fmt.Sprintf("/proc/%d/status", pid))
}

func readRSSBytesFromPath(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		// Format: "VmRSS:   12345 kB"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected VmRSS format in %s", path)
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing VmRSS in %s: %w", path, err)
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("VmRSS not found in %s", path)
}

// setPortAccess opens or closes the firewall for the given TCP port using iptables.
// open=true removes the DROP rule; open=false inserts it.
func setPortAccess(port int, open bool) error {
	portStr := strconv.Itoa(port)
	var cmd *exec.Cmd
	if open {
		cmd = exec.Command("iptables", "-D", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "DROP")
	} else {
		cmd = exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "DROP")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
