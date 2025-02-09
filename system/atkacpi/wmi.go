package atkacpi

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/NeilSeligmann/G15Manager/system/device"
	"github.com/NeilSeligmann/G15Manager/system/ioctl"
)

// Method defines the WMI method IDs
type Method uint32

// Defines the WMI method IDs (big endian for readability)
// golang: this is not ergonomic *face palm*
const (
	DSTS Method = 0x53545344
	DEVS Method = 0x53564544
	INIT Method = 0x54494e49
	BSTS Method = 0x53545342 // returns 0 on G14
	SFUN Method = 0x4e554653
)

// Defines the IIA0 argument (big endian for readability)
// golang: this is not ergonomic *face palm*
const (
	DevsHardwareCtrl       uint32 = 0x00100021
	DevsBatteryChargeLimit uint32 = 0x00120057
	DevsThrottleCtrl       uint32 = 0x00120075
	DevsCPUFanCurve        uint32 = 0x00110024
	DevsGPUFanCurve        uint32 = 0x00110025
	DstsDefaultCPUFanCurve uint32 = 0x00110024
	DstsDefaultGPUFanCurve uint32 = 0x00110025
	DstsCurrentCPUFanSpeed uint32 = 0x00110013
	DstsCurrentGPUFanSpeed uint32 = 0x00110014
	DstsCheckCharger       uint32 = 0x0012006c
)

// This is needed since we are calling from userspace
// and we need atkwmiacpi64.sys to do the leg work of
// calling ACPI methods from kernel space
// However, we could technically interact with ACPI\PNP0C14\ATK...
const devicePath = `\\.\ATKACPI`

// WMI is for evaluating WMI methods
type WMI interface {
	// Evaluate will pass through the buffer (little endian) to the WMI method
	Evaluate(id Method, args []byte) ([]byte, error)
	// Close will close the underlying IO to the hardware
	Close() error
}

type atkWmi struct {
	sync.Mutex
	alreadyClosed bool
	device        *device.Control
}

var _ WMI = &atkWmi{}

// NewWMI returns an WMI for evaluating WMI methods exposed by the ATKD ACPI device
func NewWMI(dryRun bool) (WMI, error) {
	device, err := device.NewControl(device.Config{
		DryRun:      dryRun,
		Path:        devicePath,
		ControlCode: ioctl.ATK_ACPI_WMIFUNCTION,
	})
	if err != nil {
		return nil, err
	}
	return &atkWmi{
		device: device,
	}, nil
}

func (a *atkWmi) Evaluate(id Method, args []byte) ([]byte, error) {
	a.Lock()
	defer a.Unlock()

	if len(args) < 4 {
		return nil, fmt.Errorf("args should have at least one parameter")
	}

	acpiBuf := make([]byte, 8+len(args))
	binary.LittleEndian.PutUint32(acpiBuf[0:], uint32(id))
	binary.LittleEndian.PutUint32(acpiBuf[4:], uint32(len(args)))
	copy(acpiBuf[8:], args)

	result, err := a.device.Execute(acpiBuf, 16)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (a *atkWmi) Close() error {
	a.Lock()
	defer a.Unlock()
	if a.alreadyClosed {
		return nil
	}
	a.alreadyClosed = true
	return a.device.Close()
}
