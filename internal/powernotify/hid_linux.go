//go:build linux

package powernotify

import "fmt"

func RegisterHIDInterfaceArrivalNotifications([]HIDDeviceID, func(string)) (func(), error) {
	return nil, fmt.Errorf("HID interface arrival notifications are not implemented on Linux")
}
