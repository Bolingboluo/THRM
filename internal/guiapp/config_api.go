package guiapp

import (
	"encoding/json"
	"fmt"

	"github.com/TIANLI0/THRM/internal/ipc"
)

// GetConfig 获取当前配置。
//
// 失败时必须返回错误而不是默认配置：前端会把拿到的配置存起来，之后以
// {...config, 某字段} 的形式整份写回，一次 IPC 超时就会把用户配置重置为默认值。
func (a *App) GetConfig() (AppConfig, error) {
	resp, err := a.sendRequest(ipc.ReqGetConfig, nil)
	if err != nil {
		guiLogger.Errorf("获取配置失败: %v", err)
		return AppConfig{}, err
	}
	if !resp.Success {
		guiLogger.Errorf("获取配置失败: %s", resp.Error)
		return AppConfig{}, fmt.Errorf("%s", resp.Error)
	}
	var cfg AppConfig
	if err := json.Unmarshal(resp.Data, &cfg); err != nil {
		guiLogger.Errorf("解析配置失败: %v", err)
		return AppConfig{}, err
	}
	return cfg, nil
}

// UpdateConfig 更新配置
func (a *App) UpdateConfig(cfg AppConfig) error {
	resp, err := a.sendRequest(ipc.ReqUpdateConfig, cfg)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// SetUILanguage 把界面语言告诉核心，让托盘、系统通知与快捷键提示使用同一种语言。
// 返回核心实际生效的语言。核心在语言未变时不写配置，重复上报是无害的。
func (a *App) SetUILanguage(lang string) (string, error) {
	resp, err := a.sendRequest(ipc.ReqSetUILanguage, ipc.SetStringParams{Value: lang})
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("%s", resp.Error)
	}
	var applied string
	if err := json.Unmarshal(resp.Data, &applied); err != nil {
		return "", err
	}
	return applied, nil
}

// PreviewRTSSPosition moves the live OSD without writing the application
// configuration. The final position is persisted through UpdateConfig.
func (a *App) PreviewRTSSPosition(mode string, x, y int) error {
	resp, err := a.sendRequest(ipc.ReqPreviewRTSSPosition, ipc.PreviewRTSSPositionParams{Mode: mode, X: x, Y: y})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
