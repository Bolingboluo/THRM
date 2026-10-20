package fwflash

// Chip 描述一颗 WCH 芯片的刷写相关参数（仅收录 THRM 目标设备用到的 CH59x）。
type Chip struct {
	Name       string
	ChipID     byte // identify 回包 payload[0]
	DeviceType byte // identify 回包 payload[1]，= mcuType + 0x10
	FlashSize  uint32
	EEPROMSize uint32
}

// chipDB 是已知芯片表。BS2PRO/BS3PRO 用 CH591；顺带收录 CH592 以防同族设备。
var chipDB = []Chip{
	{Name: "CH591", ChipID: 0x91, DeviceType: 0x22, FlashSize: 196608, EEPROMSize: 32768},
	{Name: "CH592", ChipID: 0x92, DeviceType: 0x22, FlashSize: 458752, EEPROMSize: 32768},
}

// findChip 按 identify 回包的 chipID + deviceType 查表。
func findChip(chipID, deviceType byte) (Chip, bool) {
	for _, c := range chipDB {
		if c.ChipID == chipID && c.DeviceType == deviceType {
			return c, true
		}
	}
	return Chip{}, false
}

// uidSize 返回参与 XOR 密钥计算的 UID 字节数。CH59x（deviceType 0x22）为 8。
func (c Chip) uidSize() int {
	if c.DeviceType == 0x11 {
		return 4
	}
	return 8
}

// minEraseSectors 返回最小擦除扇区数。CH59x 为 8。
func (c Chip) minEraseSectors() uint32 {
	if c.DeviceType == 0x10 {
		return 4
	}
	return 8
}

// supportCodeFlashProtect 返回该芯片是否支持代码区读保护位；CH59x 不在此列。
func (c Chip) supportCodeFlashProtect() bool {
	switch c.DeviceType {
	case 0x14, 0x15, 0x17, 0x18, 0x19, 0x20:
		return true
	}
	return false
}
