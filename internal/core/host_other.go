//go:build !windows

package core

// HostCPULoad is this machine's processor load: /proc/stat.
func HostCPULoad() *CPULoad { return NewCPULoad(ProcStatPath) }

// HostCPUName is this machine's processor model: /proc/cpuinfo's.
func HostCPUName() string { return CPUName(CPUInfoPath) }
