package ps

import (
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

func CPUStatus() (CPU, error) {
	list, err := cpu.Percent(time.Millisecond*50, false)
	if err != nil {
		return CPU{}, err
	}

	return CPU{
		Percent: list[0],
	}, nil
}

func MemoryStatus() (used, free, total uint64, usedPercent float64, err error) {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return
	}

	used = memory.Used
	free = memory.Free
	total = memory.Total
	usedPercent = memory.UsedPercent
	return
}

func DiskUsage(path string) (used, free, total uint64, usedPercent float64, err error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return
	}
	used = usage.Used
	free = usage.Free
	usedPercent = usage.UsedPercent
	total = usage.Total

	return
}

func DirDiskUsage(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return size, nil
}

type CPU struct {
	Percent float64
}
