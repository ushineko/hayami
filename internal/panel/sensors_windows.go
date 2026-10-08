package panel

// cpuSensorDetail says why a Windows machine has no processor temperature.
// The kernel keeps it behind a driver: the ACPI thermal zone, the one route
// without one, is unsupported on most desktop boards and reads a fixed value
// on others (spec 034).
func cpuSensorDetail() string {
	return "Windows offers no CPU temperature without a kernel driver"
}

// gpuSensorDetail is every route to the card's temperature, for the reason
// given when none answers.
func gpuSensorDetail() string {
	return "asked D3DKMT and the GPU Engine counters, and tried nvidia-smi"
}
