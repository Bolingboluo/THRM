// Package fwupdate 提供固件来源（官方 API / THRM 托管 / 本地文件）与下载，
// 配合 internal/fwflash 引擎完成 THRM 内的固件更新。
package fwupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 来源标识。
const (
	SourceOfficial = "official"
	SourceCustom   = "custom" // THRM 自定义固件（托管在 GitHub 仓库）
	SourceLocal    = "local"
)

// CustomFirmwareBaseURL 是 THRM 自定义固件清单的 GitHub 原始文件基址。
// 约定清单在 <base>/<deviceCode>.json，内容为 []FirmwareInfo。
const CustomFirmwareBaseURL = "https://raw.githubusercontent.com/TIANLI0/THRM/main/firmware"

// FirmwareInfo 描述一个可刷写的固件条目。
type FirmwareInfo struct {
	Source  string `json:"source"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Info    string `json:"info"`
	Date    string `json:"date"`
	URL     string `json:"url"`
}

// DeviceCode 把 USB ProductID 映射成飞智 API 的 device_code。
func DeviceCode(productID uint16) string {
	switch productID {
	case 0x1002:
		return "bs2pro"
	case 0x1004:
		return "bs3pro"
	case 0x1001:
		return "bs2"
	case 0x1003:
		return "bs3"
	default:
		return ""
	}
}

func httpClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

// FetchOfficial 查询飞智官方更新接口，返回该设备当前可下载的最新固件。
// 传一个很旧的 main_chip 以强制服务器返回最新包（见抓包分析）。
func FetchOfficial(ctx context.Context, deviceCode string) (*FirmwareInfo, error) {
	if deviceCode == "" {
		return nil, fmt.Errorf("未知设备，无法查询官方固件")
	}
	body, _ := json.Marshal(map[string]any{
		"device_code": deviceCode,
		"device_id":   0,
		"app_version": "",
		"main_chip":   "0.0.1.0",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.flydigi.com/pc/Update/firmware", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接官方服务器失败: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		Code int `json:"code"`
		Data struct {
			DeviceCode string `json:"device_code"`
			ChipList   struct {
				MainChip *struct {
					Version string `json:"version"`
					URL     string `json:"url"`
					Info    string `json:"info"`
				} `json:"main_chip"`
			} `json:"chip_list"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("解析官方响应失败: %w", err)
	}
	mc := out.Data.ChipList.MainChip
	if mc == nil || mc.URL == "" {
		return nil, fmt.Errorf("官方暂无该设备的固件")
	}
	return &FirmwareInfo{
		Source:  SourceOfficial,
		Version: mc.Version,
		Name:    fmt.Sprintf("%s_%s", strings.ToUpper(deviceCode), mc.Version),
		Info:    mc.Info,
		URL:     mc.URL,
	}, nil
}

// FetchCustom 从 GitHub 仓库拉该设备的 THRM 自定义固件清单。
// 约定: <base>/<deviceCode>.json 返回 []FirmwareInfo。源不可用时返回空列表而非报错。
func FetchCustom(ctx context.Context, baseURL, deviceCode string) ([]FirmwareInfo, error) {
	if baseURL == "" {
		baseURL = CustomFirmwareBaseURL
	}
	if deviceCode == "" {
		return nil, nil
	}
	url := strings.TrimRight(baseURL, "/") + "/" + deviceCode + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, nil // 源不可用不算错误
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var list []FirmwareInfo
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, nil
	}
	for i := range list {
		list[i].Source = SourceCustom
	}
	return list, nil
}

// Download 下载固件内容（.hex/.bin 原始字节）。URL 为 http 时按 https 失败再退回原值尝试。
func Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载固件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载固件失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("下载到的固件为空")
	}
	return data, nil
}
