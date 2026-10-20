// Package fwflash 是 WCH CH59x 的 USB ISP 刷写引擎（原生 Go，无 CGO）。
// 协议逐条移植自开源的 ch32-rs/wchisp，用于在 THRM 内直接刷写 BS2PRO/BS3PRO
// （CH591）固件，不依赖外部 exe、不打包 libusb。
package fwflash

// WCH ISP 命令码。
const (
	cmdIdentify    = 0xA1
	cmdISPEnd      = 0xA2
	cmdISPKey      = 0xA3
	cmdErase       = 0xA4
	cmdProgram     = 0xA5
	cmdVerify      = 0xA6
	cmdReadConfig  = 0xA7
	cmdWriteConfig = 0xA8
	cmdDataErase   = 0xA9
	cmdDataProgram = 0xAA
	cmdDataRead    = 0xAB
)

// 配置寄存器读取位掩码。
const (
	cfgMaskRDPRUserDataWPR = 0x07
	cfgMaskBTVER           = 0x08
	cfgMaskUID             = 0x10
	cfgMaskAll             = 0x1F
)

// sectorSize 是 1KiB 擦除扇区；programChunk 是每个 program 包的数据字节数。
const (
	sectorSize   = 1024
	programChunk = 56
	ispKeySeed   = 0x1e // isp_key 种子长度，全 0
)

func le16(v uint16) []byte { return []byte{byte(v), byte(v >> 8)} }
func le32(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

// buildIdentify 构造 0xA1 识别命令（初次 device_id/device_type 传 0）。
func buildIdentify(deviceID, deviceType byte) []byte {
	buf := []byte{cmdIdentify, 0x12, 0x00, deviceID, deviceType}
	return append(buf, []byte("MCU ISP & WCH.CN")...)
}

// buildISPEnd 构造 0xA2 结束/复位命令。reason=1 表示带配置生效的正常复位。
func buildISPEnd(reason byte) []byte {
	return []byte{cmdISPEnd, 0x01, 0x00, reason}
}

// buildISPKey 构造 0xA3 密钥种子命令。
func buildISPKey(key []byte) []byte {
	buf := []byte{cmdISPKey, byte(len(key)), 0x00}
	return append(buf, key...)
}

// buildErase 构造 0xA4 代码区擦除命令（按 1KiB 扇区数）。
func buildErase(sectors uint32) []byte {
	return append([]byte{cmdErase, 0x04, 0x00}, le32(sectors)...)
}

// buildReadConfig 构造 0xA7 读配置命令。
func buildReadConfig(bitMask byte) []byte {
	return []byte{cmdReadConfig, 0x02, 0x00, bitMask, 0x00}
}

// buildProgramLike 构造 0xA5/0xA6（program/verify）命令，data 已做 XOR。
func buildProgramLike(cmd byte, address uint32, padding byte, data []byte) []byte {
	size := uint16(4 + 1 + len(data)) // addr(4) + padding(1) + data
	buf := make([]byte, 0, 3+int(size))
	buf = append(buf, cmd)
	buf = append(buf, le16(size)...)
	buf = append(buf, le32(address)...)
	buf = append(buf, padding)
	buf = append(buf, data...)
	return buf
}

// xorKey 由芯片 UID 和 chipID 推出 8 字节 XOR 密钥（对应全 0 密钥种子）。
// 算法：checksum = sum(uid) 低 8 位；key=[checksum;8]；key[7] += chipID。
func xorKey(uid []byte, chipID byte) [8]byte {
	var sum byte
	for _, b := range uid {
		sum += b
	}
	var k [8]byte
	for i := range k {
		k[i] = sum
	}
	k[7] += chipID
	return k
}

// keyChecksum 是 8 字节密钥之和的低 8 位；isp_key 回包首字节应等于它。
func keyChecksum(k [8]byte) byte {
	var sum byte
	for _, b := range k {
		sum += b
	}
	return sum
}

// xorInto 用 key 对 data 逐字节异或（key 按 i%8 循环）。
func xorInto(data []byte, key [8]byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ key[i%8]
	}
	return out
}

// parseResponse 从原始回包解析出 payload。回包格式：
// [cmd][state][len_lo][len_hi][payload...]，len 为小端。
func parseResponse(reqCmd byte, raw []byte) ([]byte, error) {
	if len(raw) < 4 {
		return nil, errShortResponse
	}
	if raw[0] != reqCmd {
		return nil, errCmdMismatch
	}
	n := int(raw[2]) | int(raw[3])<<8
	if 4+n > len(raw) {
		return nil, errBadLength
	}
	return raw[4 : 4+n], nil
}
