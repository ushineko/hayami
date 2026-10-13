package core

// SetWait replaces the pause between a CPULoad's first two samples.
func (c *CPULoad) SetWait(wait func()) { c.wait = wait }

// The platforms' chains and accounts (spec 043), for a test on any system.
var (
	LinuxCPUTemperature   = linuxCPUTemperature
	LinuxCPUMissing       = linuxCPUMissing
	LinuxGraphics         = linuxGraphics
	LinuxGPUMissing       = linuxGPUMissing
	WindowsCPUTemperature = windowsCPUTemperature
	WindowsCPUMissing     = windowsCPUMissing
	WindowsGraphics       = windowsGraphics
	WindowsGPUMissing     = windowsGPUMissing
	OtherCPUMissing       = otherCPUMissing
	OtherGPUMissing       = otherGPUMissing
)
