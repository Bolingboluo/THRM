package powernotify

// HIDDeviceID 是一个 VID/PID 组合，用来指定要监听接口到达事件的 HID 设备。
//
// 之所以要能传多组：飞智的散热器分两个 VID 体系——BS2/BS3 系列是
// VID 0x37D7，BS1 是 VID 0x04A0。只登记其中一个的话，另一族的设备
// 插上线时收不到到达通知。
type HIDDeviceID struct {
	VendorID  uint16
	ProductID uint16
}
