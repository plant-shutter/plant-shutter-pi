package ps

import (
	"log"
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

func MemoryStatus() (Memory, error) {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return Memory{}, err
	}
	swapMemory, err := mem.SwapMemory()
	if err != nil {
		return Memory{}, err
	}

	return Memory{
		Total:       memory.Total,
		Used:        memory.Used,
		UsedPercent: memory.UsedPercent,

		SwapTotal:       swapMemory.Total,
		SwapUsed:        swapMemory.Used,
		SwapUsedPercent: swapMemory.UsedPercent,
	}, nil
}

func Disks() {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return
	}
	log.Println(partitions)
}

func DiskUsage(path string) (used, total uint64, usedPercent float64, err error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return
	}
	used = usage.Used
	usedPercent = usage.UsedPercent
	total = usage.Total
	log.Println(usage)
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

type Memory struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"usedPercent"`

	SwapTotal       uint64  `json:"swapTotal"`
	SwapUsed        uint64  `json:"swapUsed"`
	SwapUsedPercent float64 `json:"swapUsedPercent"`
}
