// Package theme 负责「自定义界面主题」的发现、播种与读取。
package theme

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	manifestName = "theme.json"
	styleName    = "theme.css"

	// SourceUser/SourceInstall/SourceBuiltin 标记主题来源，便于前端提示与排序。
	SourceUser    = "user"
	SourceInstall = "install"
	SourceBuiltin = "builtin"
)

// Meta 是主题清单（theme.json）解析后的元数据，同时作为返回给前端的结构。
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Base        string `json:"base"` // light | dark
	Author      string `json:"author,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // user | install | builtin
}

// Manager 统一管理主题的发现与读取。
type Manager struct {
	installDir string // 安装目录下的 themes 目录（只读来源）
	userDir    string // 用户配置目录下的 themes 目录（可写）
	builtin    fs.FS  // 程序内置的默认主题（已 Sub 到 themes 根，路径形如 thrm/theme.json）
}

// NewManager 创建主题管理器。
//   - installThemesDir：安装目录下的 themes 目录（一般与可执行文件同级）。
//   - userThemesDir：用户配置目录下的 themes 目录，用于保存可编辑副本。
//   - builtin：内置默认主题文件系统，根目录下应直接是各主题文件夹（thrm/...）。可为 nil。
func NewManager(installThemesDir, userThemesDir string, builtin fs.FS) *Manager {
	return &Manager{
		installDir: installThemesDir,
		userDir:    userThemesDir,
		builtin:    builtin,
	}
}

// validID 校验主题 id：仅允许小写字母、数字、连字符、下划线，避免被用作路径穿越或非法选择器。
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func normalizeBase(base string) string {
	if base == "dark" {
		return "dark"
	}
	return "light"
}

// EnsureSeeded 在首次运行时把内置主题播种到磁盘，方便用户直接编辑。
//
// 对每个内置主题：若安装目录与用户目录都不存在该主题，则写入用户目录。
// 安装目录始终被视为只读；失败不影响后续从内置读取。
func (m *Manager) EnsureSeeded() {
	if m.builtin == nil {
		return
	}
	entries, err := fs.ReadDir(m.builtin, ".")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if !validID(id) {
			continue
		}
		if m.themeExistsOnDisk(m.installDir, id) || m.themeExistsOnDisk(m.userDir, id) {
			continue
		}
		_ = m.copyBuiltin(id, m.userDir)
	}
}

func (m *Manager) themeExistsOnDisk(baseDir, id string) bool {
	if baseDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(baseDir, id, manifestName))
	return err == nil
}

// copyBuiltin 把内置主题 id 复制到 baseDir/id。
func (m *Manager) copyBuiltin(id, baseDir string) error {
	if baseDir == "" {
		return fmt.Errorf("目标目录为空")
	}
	srcRoot := id
	entries, err := fs.ReadDir(m.builtin, srcRoot)
	if err != nil {
		return err
	}
	dstDir := filepath.Join(baseDir, id)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue // 内置主题为扁平结构，忽略子目录
		}
		data, err := fs.ReadFile(m.builtin, srcRoot+"/"+entry.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// List 返回所有可用主题，按来源合并去重（用户 > 安装 > 内置），并按名称排序。
func (m *Manager) List() []Meta {
	merged := map[string]Meta{}

	// 来源按优先级从低到高排列，后扫描的覆盖先扫描的。
	for _, src := range m.sources() {
		for _, entry := range scanFS(src.fsys, src.source) {
			merged[entry.meta.ID] = entry.meta
		}
	}

	out := make([]Meta, 0, len(merged))
	for _, meta := range merged {
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

type themeSource struct {
	fsys   fs.FS
	source string
}

// sources 按优先级从低到高返回各主题来源：内置 < 安装目录 < 用户目录。
func (m *Manager) sources() []themeSource {
	var out []themeSource
	if m.builtin != nil {
		out = append(out, themeSource{m.builtin, SourceBuiltin})
	}
	for _, src := range []struct{ dir, source string }{
		{m.installDir, SourceInstall},
		{m.userDir, SourceUser},
	} {
		if src.dir != "" {
			out = append(out, themeSource{os.DirFS(src.dir), src.source})
		}
	}
	return out
}

// themeEntry 是一个主题清单及其所在的文件夹名。
type themeEntry struct {
	meta   Meta
	folder string
}

// scanFS 扫描某个来源根目录下的各主题文件夹。
//
// 主题以清单里的 id 为准，文件夹名只在清单缺 id 时兜底，所以 List 和 ReadCSS
// 都必须经由这里把 id 解析回文件夹：复制出来还没改 id 的文件夹会与原主题同 id，
// 按文件夹名直接拼路径会让列表展示一个主题、样式却读自另一个文件夹。
// 同一来源内同 id 时与 id 同名的文件夹优先，其余按文件夹名排序先到先得。
func scanFS(fsys fs.FS, source string) []themeEntry {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var out []themeEntry
	index := map[string]int{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(fsys, entry.Name()+"/"+manifestName)
		if err != nil {
			continue
		}
		meta, ok := parseMeta(data, entry.Name())
		if !ok {
			continue
		}
		meta.Source = source
		if i, dup := index[meta.ID]; dup {
			if entry.Name() == meta.ID {
				out[i] = themeEntry{meta, entry.Name()}
			}
			continue
		}
		index[meta.ID] = len(out)
		out = append(out, themeEntry{meta, entry.Name()})
	}
	return out
}

// parseMeta 解析 theme.json；folderName 用于在清单缺少 id 时兜底，并校验 id 合法性。
func parseMeta(data []byte, folderName string) (Meta, bool) {
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, false
	}
	if meta.ID == "" {
		meta.ID = folderName
	}
	if !validID(meta.ID) {
		return Meta{}, false
	}
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	meta.Base = normalizeBase(meta.Base)
	return meta, true
}

// ReadCSS 读取指定主题的 theme.css 内容。
// 查找顺序：用户目录 > 安装目录 > 内置，返回首个命中的内容。
func (m *Manager) ReadCSS(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("非法主题 id: %q", id)
	}

	sources := m.sources()
	for i := len(sources) - 1; i >= 0; i-- {
		for _, entry := range scanFS(sources[i].fsys, sources[i].source) {
			if entry.meta.ID != id {
				continue
			}
			if data, err := fs.ReadFile(sources[i].fsys, entry.folder+"/"+styleName); err == nil {
				return string(data), nil
			}
			break
		}
	}

	return "", fmt.Errorf("未找到主题 %q 的样式文件", id)
}

// ResolveDir 返回应优先暴露给用户编辑的 themes 目录（用于「打开主题文件夹」）。
func (m *Manager) ResolveDir() string {
	if m.installDir != "" {
		if _, err := os.Stat(m.installDir); err == nil {
			return m.installDir
		}
	}
	if m.userDir != "" {
		_ = os.MkdirAll(m.userDir, 0o755)
		return m.userDir
	}
	return m.installDir
}
