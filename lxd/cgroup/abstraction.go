package cgroup

import (
	"fmt"
)

// CGroup represents the main cgroup abstraction.
type CGroup struct {
	rw ReadWriter
}

// SetMaxProcesses applies a limit to the number of processes
func (cg *CGroup) SetMaxProcesses(max int64) error {
	// Confirm we have the controller
	version := cgControllers["pids"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	default:
		if version == V1 || version == V2 {
			if max == -1 {
				return cg.rw.Set(version, "pids", "pids.max", "max")
			}
			return cg.rw.Set(version, "pids", "pids.max", fmt.Sprintf("%d", max))
		}
	}
	return ErrUnknownVersion
}

// GetMemorySoftLimit returns the soft limit for memory
func (cg *CGroup) GetMemorySoftLimit() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.soft_limit_in_bytes")
	case V2:
		return cg.rw.Get(version, "memory", "memory.low")
	}
	return "", ErrUnknownVersion
}

// SetMemorySoftLimit set the soft limit for memory
func (cg *CGroup) SetMemorySoftLimit(softLim string) error {
	// Confirm we have the controller
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		if softLim == "-1" {
			return cg.rw.Set(version, "memory", "memory.soft_limit_in_bytes", "max")
		}
		return cg.rw.Set(version, "memory", "memory.soft_limit_in_bytes", softLim)
	case V2:
		if softLim == "-1" {
			return cg.rw.Set(version, "memory", "memory.low", "max")
		}
		return cg.rw.Set(version, "memory","memory.low", softLim)
	}

	return ErrUnknownVersion
}

// GetMaxMemory return the hard limit for memory
func (cg *CGroup) GetMaxMemory() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.limit_in_bytes")
	case V2:
		return cg.rw.Get(version, "memory", "memory.max")
	}
	return "", ErrUnknownVersion
}

// SetMemoryMaxUsage sets the hard limit for memory
func (cg *CGroup) SetMemoryMaxUsage(max string) error {
	// Confirm we have the controller
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "memory", "memory.limit_in_bytes", max)
	case V2:
		return cg.rw.Set(version, "memory","memory.max", max)
	}
	return ErrUnknownVersion
}

// GetMemoryUsage returns the current use of memory
func (cg *CGroup) GetMemoryUsage() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.usage_in_bytes")
	case V2:
		return cg.rw.Get(version, "memory", "memory.current")
	}
	return "", ErrUnknownVersion
}

// GetProcessesUsage returns the current number of pids
func (cg *CGroup) GetProcessesUsage() (string, error) {
	version := cgControllers["pids"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		fallthrough
	case V2:
		return cg.rw.Get(version, "pids", "pids.current")
	}
	return "", ErrUnknownVersion
}

// SetMemorySwapMax sets the hard limit for swap
func (cg *CGroup) SetMemorySwapMax(max string) error {
	//Confirm we have the controller
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		if max == "-1" {
			return cg.rw.Set(version, "memory","memory.memsw.limit_in_bytes", "max")
		}

		return cg.rw.Set(version, "memory","memory.memsw.limit_in_bytes", max)
	case V2:
		if max == "-1" {
			return cg.rw.Set(version, "memory","memory.swap.max", "max")
		}
		return cg.rw.Set(version, "memory","memory.swap.max", max)

	}
	return ErrUnknownVersion
}

// GetCPUAcctUsage returns the total CPU time in ns used by processes
func (cg *CGroup) GetCPUAcctUsage() (string, error) {
	version := cgControllers["cpuacct"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "cpuacct", "cpuacct.usage")
	}
	return "", ErrUnknownVersion
}

// GetMemoryMaxUsage returns the record high for memory usage
func (cg *CGroup) GetMemoryMaxUsage() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.max_usage_in_bytes")
	}
	return "", ErrUnknownVersion
}

// GetMemorySwMaxUsage returns the record high for swap usage
func (cg *CGroup) GetMemorySwMaxUsage() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.memsw.max_usage_in_bytes")
	}
	return "", ErrUnknownVersion
}

// SetMemorySwappiness sets swappiness paramet of vmscan
func (cg *CGroup) SetMemorySwappiness(value string) error {
	// Confirm we have the controller
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "memory","memory.swappiness", value)
	}
	return ErrUnknownVersion
}

// GetMemorySwapLimit returns the hard limit on swap usage
func (cg *CGroup) GetMemorySwapLimit() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.memsw.limit_in_bytes")
	case V2:
		return cg.rw.Get(version, "memory", "memory.swap.max")
	}
	return "", ErrUnknownVersion
}

// GetMemorySwapUsage return current usage of swap
func (cg *CGroup) GetMemorySwapUsage() (string, error) {
	version := cgControllers["memory"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "memory", "memory.memsw.usage_in_bytes")
	case V2:
		return cg.rw.Get(version, "memory", "memory.swap.current")
	}
	return "", ErrUnknownVersion
}

// GetBlkioWeight returns the currently allowed range of weights
func (cg *CGroup) GetBlkioWeight() (string, error) {
	// Confirm we have the controller
	version := cgControllers["blkio"]
	switch version {
	case Unavailable:
		return "", ErrControllerMissing
	case V1:
		return cg.rw.Get(version, "blkio", "blkio.weight")
	}
	return "", ErrUnknownVersion
}

// SetBlkioWeight set the currently allowed range of weights
func (cg *CGroup) SetBlkioWeight(value string) error {
	version := cgControllers["blkio"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "blkio", "blkio.weight", value)
	}
	return ErrUnknownVersion

}

// SetCPUShare sets the weight of each group in the same hierarchy
func (cg *CGroup) SetCPUShare(value string) error {
	//Confirm we have the controller
	version := cgControllers["cpu"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "cpu","cpu.shares", value)
	}
	return ErrUnknownVersion
}

// SetCPUCfsPeriod sets the duration in ms for each scheduling period
func (cg *CGroup) SetCPUCfsPeriod(value string) error {
	//Confirm we have the controller
	version := cgControllers["cpu"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "cpu","cpu.cfs_period_us", value)
	}
	return ErrUnknownVersion
}

// SetCPUCfsQuota sets the max time in ms during each cfs_period_us that
// the current group can run for
func (cg *CGroup) SetCPUCfsQuota(value string) error {
	//Confirm we have the controller
	version := cgControllers["cpu"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "cpu","cpu.cfs_quota_us", value)
	}
	return ErrUnknownVersion
}

// SetNetIfPrio sets the priority for the process
func (cg *CGroup) SetNetIfPrio(value string) error {
	version := cgControllers["net_prio"]
	switch version {
	case Unavailable:
		return ErrControllerMissing
	case V1:
		return cg.rw.Set(version, "net_prio","net_prio.ifpriomap", value)
	}
	return ErrUnknownVersion
}
