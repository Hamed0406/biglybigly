package sysmon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func collectCPU(prev *cpuSample) (float64, *cpuSample) {
	// Use PowerShell to get CPU load percentage
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average`).Output()
	if err != nil {
		return 0, prev
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, prev
	}
	return val, nil
}

func collectMemory() (total, used, available uint64, err error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_OperatingSystem | Select-Object TotalVisibleMemorySize,FreePhysicalMemory | ConvertTo-Json`).Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("powershell memory: %w", err)
	}

	var result struct {
		TotalVisibleMemorySize uint64 `json:"TotalVisibleMemorySize"`
		FreePhysicalMemory     uint64 `json:"FreePhysicalMemory"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, 0, 0, fmt.Errorf("parse memory: %w", err)
	}

	total = result.TotalVisibleMemorySize * 1024
	available = result.FreePhysicalMemory * 1024
	used = total - available
	return total, used, available, nil
}

func collectLoadAvg() (l1, l5, l15 float64) {
	// Windows doesn't have load averages
	return 0, 0, 0
}

func collectOSInfo() string {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`(Get-CimInstance Win32_OperatingSystem).Caption`).Output()
	if err != nil {
		return "Windows " + runtime.GOARCH
	}
	caption := strings.TrimSpace(string(out))
	if caption == "" {
		return "Windows"
	}
	return caption
}

func collectHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

func collectUptime() int64 {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`[int](New-TimeSpan -Start (Get-CimInstance Win32_OperatingSystem).LastBootUpTime -End (Get-Date)).TotalSeconds`).Output()
	if err != nil {
		return 0
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return secs
}

func collectDisks() ([]DiskInfo, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | Select-Object DeviceID,FileSystem,Size,FreeSpace | ConvertTo-Json`).Output()
	if err != nil {
		return nil, fmt.Errorf("powershell disks: %w", err)
	}

	// Handle single vs array JSON
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	type WinDisk struct {
		DeviceID   string `json:"DeviceID"`
		FileSystem string `json:"FileSystem"`
		Size       uint64 `json:"Size"`
		FreeSpace  uint64 `json:"FreeSpace"`
	}

	var disks []DiskInfo

	if strings.HasPrefix(trimmed, "[") {
		var arr []WinDisk
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil, fmt.Errorf("parse disks array: %w", err)
		}
		for _, d := range arr {
			disks = append(disks, DiskInfo{
				MountPoint: d.DeviceID,
				FSType:     d.FileSystem,
				TotalBytes: d.Size,
				UsedBytes:  d.Size - d.FreeSpace,
				AvailBytes: d.FreeSpace,
			})
		}
	} else {
		var d WinDisk
		if err := json.Unmarshal([]byte(trimmed), &d); err != nil {
			return nil, fmt.Errorf("parse disk: %w", err)
		}
		disks = append(disks, DiskInfo{
			MountPoint: d.DeviceID,
			FSType:     d.FileSystem,
			TotalBytes: d.Size,
			UsedBytes:  d.Size - d.FreeSpace,
			AvailBytes: d.FreeSpace,
		})
	}

	return disks, nil
}
