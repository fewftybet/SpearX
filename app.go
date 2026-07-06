package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp 创建新的 App 应用
func NewApp() *App {
	return &App{}
}

// Log 调试日志输出
func (a *App) Log(message string) error {
	Debug(message)
	return nil
}

// LogError 前端错误日志记录（自动写入 error.log）
func (a *App) LogError(context string, message string) error {
	err := fmt.Errorf("%s", message)
	LogErrorWithContext(context, err)
	return nil
}

// GetErrorLogPath 获取错误日志文件路径（用于前端展示或打开）
func (a *App) GetErrorLogPath() string {
	if logger != nil {
		return logger.GetErrorLogPath()
	}
	return ""
}

// OpenErrorLog 在文件资源管理器中定位错误日志文件
func (a *App) OpenErrorLog() error {
	logPath := a.GetErrorLogPath()
	if logPath == "" {
		return fmt.Errorf("错误日志路径未初始化")
	}
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("错误日志文件不存在: %s", logPath)
	}
	// 使用 explorer /select 定位文件并选中
	cmd := exec.Command("explorer", "/select,", logPath)
	return cmd.Start()
}

// startup 在应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 启动时自动检测和修复配置
	// 注意：禁用自动清理功能，避免误删用户手动添加的工具
	// 用户可以通过手动触发清理功能来维护配置
	go func() {
		Info("正在进行启动时配置检查和修复...")

		// 1. 修复配置文件格式问题（只修复格式，不删除工具）
		if err := a.RepairConfigFile(); err != nil {
			Error("配置文件修复失败: %v", err)
		}

		// 2. 清理和修复工具路径（只修复路径格式，不删除工具）
		// 注意：这个函数只修复路径格式，不会删除工具
		if err := a.CleanupToolPaths(); err != nil {
			Error("路径修复失败: %v", err)
		}

		// 3. 禁用自动清理重复工具和无效路径，避免误删用户手动添加的工具
		// 用户可以通过界面手动触发清理功能
		// if err := a.CleanupDuplicateTools(); err != nil {
		// 	Error("重复工具清理失败: %v", err)
		// }

		Info("启动时配置检查和修复完成")
	}()
}

// JavaConfig Java配置结构体
type JavaConfig struct {
	Java8  string `json:"Java8" yaml:"Java8"`
	Java11 string `json:"Java11" yaml:"Java11"`
	Java17 string `json:"Java17" yaml:"Java17"`
}

// Settings 应用全局设置结构体
type Settings struct {
	RunAsAdmin bool `json:"runAsAdmin" yaml:"RunAsAdmin"` // 是否以管理员方式启动 Open 类型的工具
}

// Config 配置结构体
type Config struct {
	JavaPaths  JavaConfig `yaml:"javapath"`
	Categories []Category `yaml:"Categories"`
	Settings   Settings   `yaml:"Settings"`
}

// Tool 工具结构体
type Tool struct {
	ID          string    `json:"id" yaml:"ID,omitempty"`                   // 工具唯一ID: {工具名称}-{YYYYMMDD}
	Name        string    `json:"name" yaml:"ToolName"`                     // 工具名称
	Path        string    `json:"path" yaml:"PATH"`                         // 工具路径
	FileName    string    `json:"fileName" yaml:"FileName"`                 // 文件名
	Value       string    `json:"value" yaml:"VALUE"`                       // 执行类型
	Command     string    `json:"command" yaml:"COMMAND"`                   // 命令
	Optional    string    `json:"optional" yaml:"Optional"`                 // 可选参数
	Description string    `json:"description" yaml:"Description,omitempty"` // 描述
	Tags        []string  `json:"tags" yaml:"Tags,omitempty"`               // 标签列表
	SourceURL   string    `json:"sourceUrl" yaml:"SourceURL,omitempty"`     // 来源URL
	IconPath    string    `json:"iconPath" yaml:"IconPath,omitempty"`       // 自定义图标路径
	OpenCount   int       `json:"openCount" yaml:"OpenCount,omitempty"`     // 打开次数
	RunAsAdmin  bool      `json:"runAsAdmin" yaml:"RunAsAdmin,omitempty"`   // 是否以管理员权限启动（仅对 Open 类型生效）
	CreatedAt   time.Time `json:"createdAt" yaml:"CreatedAt,omitempty"`     // 创建时间
	LastUsedAt  time.Time `json:"lastUsedAt" yaml:"LastUsedAt,omitempty"`   // 最后使用时间
}

// WebTool 网页工具结构体
type WebTool struct {
	ID          string    `json:"id" yaml:"ID"`                             // 工具唯一ID
	Name        string    `json:"name" yaml:"Name"`                         // 工具名称
	URL         string    `json:"url" yaml:"URL"`                           // 网页URL
	Description string    `json:"description" yaml:"Description,omitempty"` // 描述
	Category    string    `json:"category" yaml:"Category"`                 // 分类
	Tags        []string  `json:"tags" yaml:"Tags,omitempty"`               // 标签列表
	IconPath    string    `json:"iconPath" yaml:"IconPath,omitempty"`       // 自定义图标路径
	OpenCount   int       `json:"openCount" yaml:"OpenCount,omitempty"`     // 打开次数
	CreatedAt   time.Time `json:"createdAt" yaml:"CreatedAt,omitempty"`     // 创建时间
	LastUsedAt  time.Time `json:"lastUsedAt" yaml:"LastUsedAt,omitempty"`   // 最后使用时间
}

// WebNote 网页笔记结构体
type WebNote struct {
	ID        string    `json:"id" yaml:"ID"`                         // 笔记唯一ID
	Title     string    `json:"title" yaml:"Title"`                   // 标题
	URL       string    `json:"url" yaml:"URL,omitempty"`             // 参考URL
	Category  string    `json:"category" yaml:"Category"`             // 分类
	Tools     []string  `json:"tools" yaml:"Tools,omitempty"`         // 相关工具
	Tags      []string  `json:"tags" yaml:"Tags,omitempty"`           // 标签列表
	Content   string    `json:"content" yaml:"Content"`               // 笔记内容(Markdown)
	CreatedAt time.Time `json:"createdAt" yaml:"CreatedAt,omitempty"` // 创建时间
	UpdatedAt time.Time `json:"updatedAt" yaml:"UpdatedAt,omitempty"` // 更新时间
}

// Category 分类结构体
type Category struct {
	Name string `json:"name" yaml:"CategoryName"`
	Icon string `json:"icon" yaml:"Icon,omitempty"` // 分类图标
	Tool []Tool `json:"tools" yaml:"Tools"`
}

// Categories 分类列表结构体
type Categories struct {
	Category []Category `json:"categories" yaml:"Categories"`
}

// GetCategories 获取所有工具分类
func (a *App) GetCategories() (Categories, error) {
	// 直接从默认路径读取，避免循环依赖
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	var categories Categories

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 如果配置文件不存在，返回空的分类列表
		return categories, nil
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return categories, fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &categories); err != nil {
		return categories, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return categories, nil
}

// ExecuteCommand 执行工具命令
func (a *App) ExecuteCommand(path, optional, value, filename string) error {
	return a.ExecuteCommandWithAdmin(path, optional, value, filename, "", "", false)
}

// ExecuteCommandWithRunAs 执行工具命令（支持指定是否以管理员方式运行）
func (a *App) ExecuteCommandWithRunAs(path, optional, value, filename string, runAsAdmin bool) error {
	return a.ExecuteCommandWithAdmin(path, optional, value, filename, "", "", runAsAdmin)
}

// ExecuteCustomCommand 执行自定义命令
func (a *App) ExecuteCustomCommand(path, optional, value, filename, customCommand string) error {
	return a.ExecuteCommandWithAdmin(path, optional, value, filename, customCommand, "", false)
}

// ExecuteCustomCommandWithRunAs 执行自定义命令（支持指定是否以管理员方式运行）
func (a *App) ExecuteCustomCommandWithRunAs(path, optional, value, filename, customCommand string, runAsAdmin bool) error {
	return a.ExecuteCommandWithAdmin(path, optional, value, filename, customCommand, "", runAsAdmin)
}

// GetJavaConfig 获取Java配置
func (a *App) GetJavaConfig() (*JavaConfig, error) {
	// 读取配置文件
	var config Config
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &config.JavaPaths, nil
}

// GetDefaultJavaVersion 获取默认Java版本（按 Java17 > Java11 > Java8 > "Java8" 顺序）
func (a *App) GetDefaultJavaVersion() (string, error) {
	javaConfig, err := a.GetJavaConfig()
	if err != nil {
		return "Java8", err
	}
	return a.resolveDefaultJavaVersion(javaConfig), nil
}

// resolveDefaultJavaVersion 解析默认Java版本：优先使用已配置路径的版本
func (a *App) resolveDefaultJavaVersion(javaConfig *JavaConfig) string {
	if javaConfig == nil {
		return "Java8"
	}
	if javaConfig.Java17 != "" {
		return "Java17"
	}
	if javaConfig.Java11 != "" {
		return "Java11"
	}
	if javaConfig.Java8 != "" {
		return "Java8"
	}
	return "Java8"
}

// getDefaultJavaVersion 内部方法：获取默认Java版本，失败时回退到 Java8
func (a *App) getDefaultJavaVersion() string {
	javaConfig, err := a.GetJavaConfig()
	if err != nil {
		return "Java8"
	}
	return a.resolveDefaultJavaVersion(javaConfig)
}

// SaveJavaConfig 保存Java配置
func (a *App) SaveJavaConfig(javaConfig JavaConfig) error {
	// 读取现有配置
	var config Config
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 更新Java配置
	config.JavaPaths = javaConfig

	// 写回配置文件
	data, err = yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// GetSettings 获取应用全局设置
func (a *App) GetSettings() (Settings, error) {
	var config Config
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		// 配置文件不存在时返回默认设置
		if os.IsNotExist(err) {
			return Settings{RunAsAdmin: false}, nil
		}
		return Settings{}, fmt.Errorf("读取配置文件失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Settings{RunAsAdmin: false}, nil // 解析失败也返回默认
	}
	return config.Settings, nil
}

// SaveSettings 保存应用全局设置
func (a *App) SaveSettings(settings Settings) error {
	// 读取现有配置
	var config Config
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		// 配置文件不存在时创建新配置
		if os.IsNotExist(err) {
			config = Config{
				Settings: settings,
			}
		} else {
			return fmt.Errorf("读取配置文件失败: %v", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("解析配置文件失败: %v", err)
		}
		// 更新设置
		config.Settings = settings
	}

	// 写回配置文件
	data, err = yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// ExecuteToolCommand 执行工具命令（使用工具对象）
func (a *App) ExecuteToolCommand(tool *Tool, customCommand string) error {
	runAsAdmin := false
	if tool != nil {
		runAsAdmin = tool.RunAsAdmin
	}
	return a.ExecuteCommandWithAdmin(tool.Path, tool.Optional, tool.Value, tool.FileName, customCommand, "", runAsAdmin)
}

// ExecuteCommandWithAdmin 执行命令（支持管理员权限）
func (a *App) ExecuteCommandWithAdmin(path, optional, value, filename, customCommand, javaPath string, runAsAdmin bool) error {
	Debug("ExecuteCommandWithAdmin: path=%q, filename=%q, value=%q, optional=%q, customCommand=%q, runAsAdmin=%v",
		path, filename, value, optional, customCommand, runAsAdmin)
	return a.executeCommandInternal(path, optional, value, filename, customCommand, javaPath, runAsAdmin)
}

// ExecuteCommandWithCustom 执行工具命令（支持自定义命令，保持向后兼容）
func (a *App) ExecuteCommandWithCustom(path, optional, value, filename, customCommand, javaPath string) error {
	return a.executeCommandInternal(path, optional, value, filename, customCommand, javaPath, false)
}

// executeCommandInternal 内部统一执行逻辑
func (a *App) executeCommandInternal(path, optional, value, filename, customCommand, javaPath string, runAsAdmin bool) error {
	Debug("executeCommandInternal: path=%q, filename=%q, value=%q, optional=%q, customCommand=%q, runAsAdmin=%v",
		path, filename, value, optional, customCommand, runAsAdmin)

	// 对于浏览器打开类型，保持URL原样，其他类型再进行路径清理
	if strings.ToLower(value) != "browser" {
		// 清理路径，防止路径错误
		cleanedPath := a.cleanToolPath(path)
		if cleanedPath != path {
			Debug("执行时路径已清理: %s -> %s", path, cleanedPath)
			path = cleanedPath
		}
	}

	// Validate inputs - prevent // path errors
	if strings.ToLower(value) != "browser" {
		if path == "" && filename == "" {
			err := fmt.Errorf("工具路径和文件名不能同时为空")
			LogErrorWithContext("ExecuteCommandWithCustom", err)
			return err
		}
	}
	// 读取配置文件
	var config Config
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 构建完整可执行文件路径：fileName 可能是绝对路径，也可能是相对路径
	var fullExePath string
	if filename != "" {
		if filepath.IsAbs(filename) {
			// fileName 已是完整绝对路径，直接使用
			fullExePath = filepath.Clean(filename)
		} else if filepath.IsAbs(path) {
			// path 是绝对目录，fileName 是相对文件名
			fullExePath = filepath.Clean(filepath.Join(path, filename))
		} else if path != "" {
			// 都是相对路径，基于 resources 解析
			toolPath, err := a.GetToolAbsolutePath(path, "")
			if err != nil {
				return fmt.Errorf("获取工具路径失败: %v", err)
			}
			fullExePath = filepath.Clean(filepath.Join(toolPath, filename))
		} else {
			// path 为空但 filename 不为空
			// 先在 resources 目录和当前目录查找
			resourceBase := a.getResourcePath()
			candidate1 := filepath.Join(resourceBase, "resources", filename)
			candidate2 := filepath.Join(resourceBase, filename)

			if _, err := os.Stat(candidate1); err == nil {
				fullExePath = candidate1
			} else if _, err := os.Stat(candidate2); err == nil {
				fullExePath = candidate2
			} else {
				// 默认使用 resources 下的路径
				fullExePath = candidate1
			}
		}
	}

	// 工具所在目录（用于 openterm 和打开目录功能）
	var toolDir string
	if filepath.IsAbs(path) {
		toolDir = filepath.Clean(path)
	} else if fullExePath != "" {
		toolDir = filepath.Dir(fullExePath)
	} else {
		toolDir, _ = a.GetToolAbsolutePath(path, "")
	}

	Debug("fullExePath=%s, toolDir=%s, value=%s", fullExePath, toolDir, value)

	var execCommand string
	switch value {
	case "Java8", "Java11", "Java17":
		// 构建Java可执行文件路径
		var javaExec string

		// 使用全局配置文件中的Java路径
		if value == "Java8" && config.JavaPaths.Java8 != "" {
			javaExec = config.JavaPaths.Java8
		} else if value == "Java11" && config.JavaPaths.Java11 != "" {
			javaExec = config.JavaPaths.Java11
		} else if value == "Java17" && config.JavaPaths.Java17 != "" {
			javaExec = config.JavaPaths.Java17
		} else {
			// 如果没有配置路径，使用系统java
			javaExec = "java"
		}

		Debug("JAR文件: %s", fullExePath)
		Debug("Java可执行文件: %s", javaExec)
		Debug("可选参数: %s", optional)

		// 检查Java可执行文件是否存在（仅当不是系统java时）
		if javaExec != "java" {
			if _, err := os.Stat(javaExec); err != nil {
				errMsg := fmt.Errorf("java可执行文件不存在: %s", javaExec)
				LogErrorWithContext("ExecuteCommandWithCustom.JavaCheck", errMsg)
				return errMsg
			}
		}

		// 检查JAR文件是否存在
		if _, err := os.Stat(fullExePath); err != nil {
			errMsg := fmt.Errorf("JAR文件不存在: %s", fullExePath)
			LogErrorWithContext("ExecuteCommandWithCustom.JarCheck", errMsg)
			return errMsg
		}

		// 构建执行命令：java [optional] -jar xxx.jar
		// 使用 cmd /C cd 到工具目录然后启动 java，避免 cmd /C start "" 的问题
		javaArgs := []string{}
		if optional != "" {
			javaArgs = append(javaArgs, optional)
		}
		javaArgs = append(javaArgs, "-jar", fullExePath)

		Debug("启动Java: javaExec=%s, args=%v, toolDir=%s", javaExec, javaArgs, toolDir)
		// 使用 cd /d 到工具目录然后启动 java（兼容中文路径和空格）
		if toolDir != "" {
			execCommand = fmt.Sprintf(`cd /d "%s" && "%s" %s -jar "%s"`, toolDir, javaExec, optional, fullExePath)
		} else {
			execCommand = fmt.Sprintf(`"%s" %s -jar "%s"`, javaExec, optional, fullExePath)
		}

	case "Open":
		// Windows下使用 cmd /C start "" 启动关联程序
		// 注意：管理员权限启动不通过 cmd 启动，而是由 switch value 中的 Open 分支直接 exec.Command
		// （避免 cmd shell 包装带来的引号转义和 requires elevation 问题）
		execCommand = fmt.Sprintf(`start "" "%s"`, fullExePath)

	case "openterm":
		// 检查是否有自定义命令
		if customCommand != "" {
			// 有自定义命令，替换占位符
			command := customCommand
			if fullExePath != "" {
				command = strings.ReplaceAll(command, "{file}", fullExePath)
				command = strings.ReplaceAll(command, "{filename}", filepath.Base(fullExePath))
			}
			command = strings.ReplaceAll(command, "{path}", toolDir)

			Debug("终端自定义命令: %s", command)
			Debug("工具目录: %s", toolDir)

			// 在终端中执行自定义命令
			return openTerminal(toolDir, command)
		} else {
			// 没有自定义命令，默认打开终端
			return openTerminal(toolDir)
		}
	case "Browser":
		// 直接使用系统默认浏览器打开URL或文件
		target := ""
		// 如果是URL，直接打开
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			target = path
		} else {
			target = fullExePath
		}
		execCommand = fmt.Sprintf(`start "" "%s"`, target)
		// 保存目标路径用于直接启动
		fullExePath = target

	default:
		return fmt.Errorf("不支持的命令类型: %s", value)
	}

	Debug("final execCommand=%s", execCommand)
	Info("执行命令: %s", execCommand)

	var cmd *exec.Cmd
	var javaArgsHolder []string // 用于 Java 类型的参数列表
	switch runtime.GOOS {
	case "windows":
		// 在 Windows 下，Open/Browser 类型直接使用 exec.Command 启动文件（不经过 cmd shell），
		// 避免 cmd /C start "" 在某些场景（如需要管理员权限的 exe）下失败的问题
		switch value {
		case "Open", "Browser":
			// 直接使用 exec.Command 启动目标，不经过 cmd shell
			// 这样不会出现 cmd 窗口，也不会出现引号转义问题
			if fullExePath != "" {
				Debug("直接启动目标: %s (runAsAdmin=%v)", fullExePath, runAsAdmin)
				cmd = exec.Command(fullExePath)
				// 不设置 HideWindow — 让子进程显示自己的窗口
				// 如果父进程已管理员，子进程继承权限；
				// 如果 exe manifest 要求 requireAdministrator，Windows 会自动弹出 UAC
			} else {
				cmd = exec.Command("cmd", "/C", execCommand)
			}
		case "Java8", "Java11", "Java17":
			// Java 类型：使用 cmd /C start 启动 java
			// 通过 HideWindow + CREATE_NO_WINDOW 隐藏 cmd 窗口
			// start /B 让 java 不创建新窗口且脱离 cmd 进程组，独立运行
			var javaExec string
			if value == "Java8" && config.JavaPaths.Java8 != "" {
				javaExec = config.JavaPaths.Java8
			} else if value == "Java11" && config.JavaPaths.Java11 != "" {
				javaExec = config.JavaPaths.Java11
			} else if value == "Java17" && config.JavaPaths.Java17 != "" {
				javaExec = config.JavaPaths.Java17
			} else {
				javaExec = "java"
			}
			javaArgs := []string{}
			if optional != "" {
				javaArgs = append(javaArgs, optional)
			}
			javaArgs = append(javaArgs, "-jar", fullExePath)
			Debug("启动Java: javaExec=%s, args=%v, toolDir=%s", javaExec, javaArgs, toolDir)
			// 构建 start 命令：start "" /B 让 java 进程脱离 cmd 进程组
			var startCmd string
			if toolDir != "" {
				startCmd = fmt.Sprintf(`start "" /D "%s" /B "%s" %s -jar "%s"`,
					toolDir, javaExec, optional, fullExePath)
			} else {
				startCmd = fmt.Sprintf(`start "" /B "%s" %s -jar "%s"`,
					javaExec, optional, fullExePath)
			}
			execCommand = startCmd
			// 使用 cmd /C 启动 + HideWindow + CREATE_NO_WINDOW 隐藏 cmd 窗口
			// 使用 CmdLine 绕过 Go 的引号转义
			cmd = exec.Command("cmd", "/C", startCmd)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow:    true,
				CreationFlags: 0x08000000, // CREATE_NO_WINDOW
				CmdLine:       `cmd /C ` + startCmd,
			}
			javaArgsHolder = javaArgs
		default:
			cmd = exec.Command("cmd", "/C", execCommand)
		}
	default:
		cmd = exec.Command("sh", "-c", execCommand)
	}

	// 设置标准输出和错误输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	// 执行命令
	if err := cmd.Start(); err != nil {
		errMsg := fmt.Errorf("执行命令失败: %v\n命令: %s\n标准输出: %s\n错误输出: %s",
			err, execCommand, stdout.String(), stderr.String())
		LogErrorWithContext("ExecuteCommandWithCustom.Start", errMsg)
		return errMsg
	}

	// 对于 Open/Browser/Java 类型不等待子进程结束（避免 GUI/Java 进程阻塞）
	if value == "Open" || value == "Browser" ||
		value == "Java8" || value == "Java11" || value == "Java17" {
		// 异步释放
		go func() {
			_ = cmd.Wait()
		}()
		Info("命令已启动（GUI/Java 类型不等待子进程）: %v", javaArgsHolder)
		return nil
	}

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		// 对于 Windows 下 start 启动的 GUI 程序，exit status 非 0 不视为错误
		if runtime.GOOS == "windows" && value == "Open" {
			Debug("Open命令返回（GUI应用已在后台启动）: %v", err)
		} else {
			errMsg := fmt.Errorf("命令执行出错: %v\n命令: %s\n标准输出: %s\n错误输出: %s",
				err, execCommand, stdout.String(), stderr.String())
			LogErrorWithContext("ExecuteCommandWithCustom.Wait", errMsg)
			return errMsg
		}
	}

	// 输出执行结果
	Info("命令执行完成\n标准输出: %s\n错误输出: %s",
		stdout.String(), stderr.String())

	return nil
}

// openTerminal 打开终端的辅助函数（仅支持 Windows）
func openTerminal(dir string, initialCommand ...string) error {
	var commandToRun string
	if len(initialCommand) > 0 && initialCommand[0] != "" {
		commandToRun = fmt.Sprintf("cd /d %s && %s", dir, initialCommand[0])
	} else {
		commandToRun = fmt.Sprintf("cd /d %s && dir", dir)
	}
	cmd := exec.Command("cmd", "/C", "start", "cmd", "/K", commandToRun)
	return cmd.Start()
}

// AddTool 添加新工具
func (a *App) AddTool(tool Tool, categoryName string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	var categories Categories
	var config Config

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		// 如果配置文件不存在，创建默认配置
		if os.IsNotExist(err) {
			Info("配置文件不存在，创建默认配置: %s", configPath)
			// 创建默认配置
			config = Config{
				JavaPaths: JavaConfig{
					Java8:  "resources/java8/bin/java",
					Java11: "resources/java11/bin/java",
					Java17: "resources/java17/bin/java",
				},
				Categories: []Category{},
			}
			categories = Categories{
				Category: []Category{},
			}
		} else {
			return fmt.Errorf("读取配置文件失败: %v", err)
		}
	} else {
		// 解析Categories和Config
		if err := yaml.Unmarshal(data, &categories); err != nil {
			return fmt.Errorf("解析Categories失败: %v", err)
		}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("解析Config失败: %v", err)
		}
	}

	// 检查工具名称是否已存在
	for _, category := range categories.Category {
		for _, existingTool := range category.Tool {
			if existingTool.Name == tool.Name {
				return fmt.Errorf("工具名称 '%s' 已存在", tool.Name)
			}
		}
	}

	// 查找分类并添加工具
	categoryFound := false
	for i, category := range categories.Category {
		if category.Name == categoryName {
			categories.Category[i].Tool = append(categories.Category[i].Tool, tool)
			categoryFound = true
			break
		}
	}

	if !categoryFound {
		// 如果分类不存在，创建新分类
		newCategory := Category{
			Name: categoryName,
			Tool: []Tool{tool},
		}
		categories.Category = append(categories.Category, newCategory)
	}

	// 使用统一的保存方法
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return err
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "tool-added", true)
	return nil
}

// getResourcePath 获取 resources 子目录（项目内置的预置数据）
// 注意：运行时配置/缓存/日志统一存放在 data 目录（见 getDataDir）
func (a *App) getResourcePath() string {
	projectRoot := a.findProjectRoot()
	if projectRoot == "" {
		if execPath, err := os.Executable(); err == nil {
			return filepath.Dir(execPath)
		}
		return "."
	}
	resourcesPath := filepath.Join(projectRoot, "resources")
	_ = os.MkdirAll(resourcesPath, 0755)
	return resourcesPath
}

// getDataDir 获取项目根目录的 data 子目录
// 项目根 = 包含 go.mod 的目录
// 无论开发模式（exe 在 temp）还是生产模式（exe 在 build/bin/），都能定位到同一个目录
func (a *App) getDataDir() string {
	projectRoot := a.findProjectRoot()
	if projectRoot == "" {
		if execPath, err := os.Executable(); err == nil {
			projectRoot = filepath.Dir(execPath)
		} else {
			projectRoot = "."
		}
	}
	dataDir := filepath.Join(projectRoot, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		dataDir = "data"
		_ = os.MkdirAll(dataDir, 0755)
	}
	return dataDir
}

// findProjectRoot 向上查找包含 go.mod 的目录
func (a *App) findProjectRoot() string {
	// 优先级 1: 从可执行文件位置向上查找
	if execPath, err := os.Executable(); err == nil {
		dir := filepath.Dir(execPath)
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// 优先级 2: 从当前工作目录向上查找
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

// DeleteTool 删除工具
func (a *App) DeleteTool(toolName, categoryName string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	var categories Categories
	var config Config

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并删除工具
	toolFound := false
	for i, category := range categories.Category {
		if category.Name == categoryName {
			for j, tool := range category.Tool {
				if tool.Name == toolName {
					// 删除工具
					categories.Category[i].Tool = append(
						categories.Category[i].Tool[:j],
						categories.Category[i].Tool[j+1:]...,
					)
					toolFound = true
					break
				}
			}
			if toolFound {
				break
			}
		}
	}

	if !toolFound {
		return fmt.Errorf("未找到工具: %s", toolName)
	}

	// 使用统一的保存方法
	return a.saveCategoriesToFile(categories, config)
}

// OpenToolDirectory 打开工具所在目录
func (a *App) OpenToolDirectory(path string) error {
	var fullPath string

	// 判断是绝对路径还是相对路径
	if filepath.IsAbs(path) {
		// 绝对路径，直接使用
		fullPath = path
	} else {
		// 相对路径，构建基于resources的完整路径
		fullPath = filepath.Join(a.getResourcePath(), path)
	}

	// Windows下使用 explorer 打开目录
	cleanedPath := filepath.Clean(fullPath)
	cmd := exec.Command("explorer", cleanedPath)
	return cmd.Start()
}

// GetToolTypes 获取支持的工具类型
func (a *App) GetToolTypes() []string {
	return []string{"Java8", "Java11", "Open", "Browser", "openterm"}
}

// GetToolAbsolutePath 获取工具的绝对路径
func (a *App) GetToolAbsolutePath(toolPath, fileName string) (string, error) {
	if toolPath == "" {
		return "", fmt.Errorf("工具路径不能为空")
	}

	// 对于URL类型的路径，直接返回
	if strings.HasPrefix(toolPath, "http://") || strings.HasPrefix(toolPath, "https://") {
		return toolPath, nil
	}

	var fullPath string

	// 判断是绝对路径还是相对路径
	if filepath.IsAbs(toolPath) {
		// 绝对路径，直接使用
		fullPath = toolPath
	} else {
		// 相对路径，构建基于resources的完整路径
		basePath := a.getResourcePath()
		fullPath = filepath.Join(basePath, toolPath)
	}

	// 如果有文件名，添加文件名
	if fileName != "" {
		fullPath = filepath.Join(fullPath, fileName)
	}

	// 返回清理后的绝对路径
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("获取绝对路径失败: %v", err)
	}

	return absPath, nil
}

// GetFilePath 获取文件的完整路径
func (a *App) GetFilePath(fileName string) (string, error) {
	// 这里可以根据需要处理文件路径
	// 例如，如果文件在特定目录下：
	return filepath.Abs(fileName)
}

// GetFileInfo 获取文件信息
func (a *App) GetFileInfo(filePath string) (map[string]string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %v", err)
	}

	dir := filepath.Dir(absPath)
	fileName := filepath.Base(absPath)

	return map[string]string{
		"path":     dir,
		"fileName": fileName,
		"fullPath": absPath,
	}, nil
}

// OpenFileDialog 打开文件选择对话框
func (a *App) OpenFileDialog() (map[string]string, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		DefaultDirectory: "",
		DefaultFilename:  "",
		Title:            "选择工具文件",
		// 不设置Filters，这样可以选择任意文件包括二进制文件
	})

	if err != nil {
		return nil, fmt.Errorf("选择文件失败: %v", err)
	}

	if filePath == "" {
		return nil, nil
	}

	return a.GetFileInfo(filePath)
}

// OpenDirectoryDialog 打开目录选择对话框
func (a *App) OpenDirectoryDialog() (string, error) {
	dirPath, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		DefaultDirectory: "",
		Title:            "选择工具目录",
	})

	if err != nil {
		return "", fmt.Errorf("选择目录失败: %v", err)
	}

	return dirPath, nil
}

// UpdateTool 更新工具信息
func (a *App) UpdateTool(originalName, categoryName string, tool Tool) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并更新工具
	found := false
	var originalTool Tool
	for i, category := range categories.Category {
		for j, t := range category.Tool {
			if t.Name == originalName {
				originalTool = t // 保存原始工具信息
				if categoryName == category.Name {
					// 如果在同一分类中，直接更新工具
					found = true
					categories.Category[i].Tool[j] = tool
					break
				} else {
					// 如果在不同分类中，从原分类删除
					categories.Category[i].Tool = append(
						categories.Category[i].Tool[:j],
						categories.Category[i].Tool[j+1:]...)
				}
			}
		}
		if found {
			break
		}
	}

	// 如果没有在原分类中找到或需要移动到新分类，则添加到目标分类
	if !found {
		for i, category := range categories.Category {
			if category.Name == categoryName {
				categories.Category[i].Tool = append(categories.Category[i].Tool, tool)
				break
			}
		}
	}

	// 如果工具名称发生了变化，需要重命名对应的笔记文件
	if found && originalName != tool.Name && originalTool.Path != "" {
		if err := a.renameToolNote(originalTool.Path, originalName, tool.Name); err != nil {
			// 笔记重命名失败不应该阻止工具更新，只记录错误
			Error("重命名笔记文件失败: %v", err)
		}
	}

	// 使用统一的保存方法
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return err
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "tool-updated", true)
	return nil
}

// AddCategory 添加新分类
func (a *App) AddCategory(categoryName string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 检查分类是否已存在
	for _, category := range categories.Category {
		if category.Name == categoryName {
			return fmt.Errorf("分类 '%s' 已存在", categoryName)
		}
	}

	// 添加新分类
	newCategory := Category{
		Name: categoryName,
		Tool: []Tool{}, // 空的工具列表
	}
	categories.Category = append(categories.Category, newCategory)

	// 使用统一的保存方法
	return a.saveCategoriesToFile(categories, config)
}

// DeleteCategory 删除分类及其下的所有工具
func (a *App) DeleteCategory(categoryName string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并删除分类
	found := false
	for i, category := range categories.Category {
		if category.Name == categoryName {
			// 删除分类
			categories.Category = append(
				categories.Category[:i],
				categories.Category[i+1:]...,
			)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("分类 '%s' 不存在", categoryName)
	}

	// 使用统一的保存方法
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return err
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "category-deleted", true)
	return nil
}

// UpdateCategoryTools 批量更新分类下工具顺序
func (a *App) UpdateCategoryTools(categoryName string, tools []Tool) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	var categories Categories
	var config Config

	// 读取配置文件
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 同时解析Categories和Config结构
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 找到对应分类并更新工具顺序
	for i, category := range categories.Category {
		if category.Name == categoryName {
			categories.Category[i].Tool = tools
			break
		}
	}

	// 使用统一的保存方法
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return err
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "tool-updated", true)
	return nil
}

// UpdateToolDescription 更新工具描述
func (a *App) UpdateToolDescription(toolName, categoryName, description string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并更新工具描述
	toolFound := false
	for i, category := range categories.Category {
		if category.Name == categoryName {
			for j, tool := range category.Tool {
				if tool.Name == toolName {
					// 只更新描述字段
					categories.Category[i].Tool[j].Description = description
					toolFound = true
					break
				}
			}
			if toolFound {
				break
			}
		}
	}

	if !toolFound {
		return fmt.Errorf("未找到工具: %s", toolName)
	}

	// 使用统一的保存方法
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return err
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "tool-updated", true)
	return nil
}

// SearchTools 搜索工具（支持标签搜索）
func (a *App) SearchTools(query string) ([]Tool, error) {
	categories, err := a.GetCategories()
	if err != nil {
		return nil, err
	}

	var results []Tool
	queryLower := strings.ToLower(query)

	// 检查是否是标签搜索
	isTagSearch := strings.HasPrefix(queryLower, "标签:")
	if isTagSearch {
		tagQuery := strings.TrimSpace(strings.TrimPrefix(queryLower, "标签:"))

		for _, category := range categories.Category {
			for _, tool := range category.Tool {
				for _, tag := range tool.Tags {
					if strings.Contains(strings.ToLower(tag), tagQuery) {
						results = append(results, tool)
						break
					}
				}
			}
		}
	} else {
		// 普通搜索
		for _, category := range categories.Category {
			for _, tool := range category.Tool {
				if strings.Contains(strings.ToLower(tool.Name), queryLower) ||
					strings.Contains(strings.ToLower(tool.Description), queryLower) ||
					strings.Contains(strings.ToLower(tool.Path), queryLower) ||
					strings.Contains(strings.ToLower(tool.SourceURL), queryLower) {
					results = append(results, tool)
				}
			}
		}
	}

	return results, nil
}

// GetAllTags 获取所有标签
func (a *App) GetAllTags() ([]string, error) {
	categories, err := a.GetCategories()
	if err != nil {
		return nil, err
	}

	tagSet := make(map[string]bool)

	// 收集工具标签
	for _, category := range categories.Category {
		for _, tool := range category.Tool {
			for _, tag := range tool.Tags {
				tagSet[tag] = true
			}
		}
	}

	var tags []string
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	return tags, nil
}

// GetToolNote 获取工具笔记 (新版本：从工具文件夹中读取)
func (a *App) GetToolNote(toolPath, toolName string) (string, error) {
	if toolPath == "" {
		return "", fmt.Errorf("工具路径不能为空")
	}

	// 获取工具的绝对路径
	toolDir, err := a.GetToolAbsolutePath(toolPath, "")
	if err != nil {
		return "", fmt.Errorf("获取工具路径失败: %v", err)
	}
	noteFile := filepath.Join(toolDir, fmt.Sprintf("%s.md", toolName))

	// 如果文件不存在，尝试查找旧位置的笔记并迁移
	if _, err := os.Stat(noteFile); os.IsNotExist(err) {
		// 尝试从旧的notes目录查找并迁移
		if content := a.findAndMigrateOldNote(toolPath, toolName); content != "" {
			return content, nil
		}

		// 尝试查找同目录下的其他.md文件（可能是旧名称的笔记）
		if content := a.findOtherNotesInToolDir(toolDir, toolName); content != "" {
			return content, nil
		}

		return "", nil
	}

	data, err := ioutil.ReadFile(noteFile)
	if err != nil {
		return "", fmt.Errorf("读取笔记失败: %v", err)
	}

	return string(data), nil
}

// SaveToolNote 保存工具笔记 (新版本：保存到工具文件夹中)
func (a *App) SaveToolNote(toolPath, toolName, content string) error {
	if toolPath == "" {
		return fmt.Errorf("工具路径不能为空")
	}

	// 获取工具的绝对路径
	toolDir, err := a.GetToolAbsolutePath(toolPath, "")
	if err != nil {
		return fmt.Errorf("获取工具路径失败: %v", err)
	}

	// 确保工具目录存在
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return fmt.Errorf("创建工具目录失败: %v", err)
	}

	noteFile := filepath.Join(toolDir, fmt.Sprintf("%s.md", toolName))
	return ioutil.WriteFile(noteFile, []byte(content), 0644)
}

// DeleteToolNote 删除工具笔记 (新版本：从工具文件夹中删除)
func (a *App) DeleteToolNote(toolPath, toolName string) error {
	if toolPath == "" {
		return nil // 路径为空，无需删除
	}

	// 获取工具的绝对路径
	toolDir, err := a.GetToolAbsolutePath(toolPath, "")
	if err != nil {
		return fmt.Errorf("获取工具路径失败: %v", err)
	}
	noteFile := filepath.Join(toolDir, fmt.Sprintf("%s.md", toolName))

	// 检查文件是否存在
	if _, err := os.Stat(noteFile); os.IsNotExist(err) {
		return nil // 文件不存在，不需要删除
	}

	return os.Remove(noteFile)
}

// findAndMigrateOldNote 查找并迁移旧位置的笔记
func (a *App) findAndMigrateOldNote(toolPath, toolName string) string {
	// 尝试从旧的notes目录查找笔记
	notesDir := filepath.Join(a.getResourcePath(), "notes")

	// 生成可能的旧笔记ID
	pathParts := strings.Split(toolPath, "/")
	if len(pathParts) > 0 {
		toolDirName := pathParts[len(pathParts)-1]
		possibleIds := []string{
			toolDirName,
			strings.ReplaceAll(toolDirName, " ", "_"),
			strings.ReplaceAll(toolDirName, "-", "_"),
		}

		for _, oldId := range possibleIds {
			oldNoteFile := filepath.Join(notesDir, fmt.Sprintf("%s.md", oldId))
			if data, err := ioutil.ReadFile(oldNoteFile); err == nil {
				// 找到旧笔记，迁移到新位置
				content := string(data)
				if err := a.SaveToolNote(toolPath, toolName, content); err == nil {
					// 迁移成功，删除旧文件
					os.Remove(oldNoteFile)
					Info("已迁移笔记: %s -> %s/%s.md", oldNoteFile, toolPath, toolName)
					return content
				}
			}
		}
	}

	return ""
}

// findOtherNotesInToolDir 在工具目录中查找其他笔记文件
func (a *App) findOtherNotesInToolDir(toolDir, currentToolName string) string {
	// 读取工具目录中的所有文件
	files, err := ioutil.ReadDir(toolDir)
	if err != nil {
		return ""
	}

	// 查找.md文件
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
			// 排除当前工具名称的笔记文件（避免无限循环）
			expectedFileName := fmt.Sprintf("%s.md", currentToolName)
			if file.Name() != expectedFileName {
				// 找到其他笔记文件，尝试读取内容
				noteFile := filepath.Join(toolDir, file.Name())
				if data, err := ioutil.ReadFile(noteFile); err == nil {
					content := string(data)

					// 将找到的笔记迁移到正确的文件名
					// 计算相对路径
					relativePath := strings.TrimPrefix(toolDir, filepath.Join(a.getResourcePath(), ""))
					if err := a.SaveToolNote(relativePath, currentToolName, content); err == nil {
						// 迁移成功，删除旧文件
						os.Remove(noteFile)
						Info("已迁移笔记: %s -> %s", noteFile, filepath.Join(toolDir, expectedFileName))
						return content
					}
				}
			}
		}
	}

	return ""
}

// renameToolNote 重命名工具笔记文件
func (a *App) renameToolNote(toolPath, oldName, newName string) error {
	if toolPath == "" || oldName == "" || newName == "" {
		return nil // 参数为空，无需处理
	}

	// 构建笔记文件路径
	toolDir := filepath.Join(a.getResourcePath(), toolPath)
	oldNoteFile := filepath.Join(toolDir, fmt.Sprintf("%s.md", oldName))
	newNoteFile := filepath.Join(toolDir, fmt.Sprintf("%s.md", newName))

	// 检查旧笔记文件是否存在
	if _, err := os.Stat(oldNoteFile); os.IsNotExist(err) {
		return nil // 旧笔记不存在，无需重命名
	}

	// 检查新笔记文件是否已存在
	if _, err := os.Stat(newNoteFile); err == nil {
		// 新笔记文件已存在，询问是否覆盖或合并
		// 为了安全起见，我们创建一个备份
		backupFile := filepath.Join(toolDir, fmt.Sprintf("%s_backup_%d.md", newName, time.Now().Unix()))
		if err := os.Rename(newNoteFile, backupFile); err != nil {
			return fmt.Errorf("备份现有笔记失败: %v", err)
		}
		Info("现有笔记已备份为: %s", backupFile)
	}

	// 重命名笔记文件
	if err := os.Rename(oldNoteFile, newNoteFile); err != nil {
		return fmt.Errorf("重命名笔记文件失败: %v", err)
	}

	Info("已重命名笔记: %s -> %s", oldNoteFile, newNoteFile)
	return nil
}

// ScannedTool 扫描到的工具信息
type ScannedTool struct {
	Path          string   `json:"path"`          // 工具相对路径
	Category      string   `json:"category"`      // 分类名称
	PossibleFiles []string `json:"possibleFiles"` // 可能的可执行文件列表
}

// ScanResourcesForTools 扫描resources文件夹寻找工具
func (a *App) ScanResourcesForTools() ([]ScannedTool, error) {
	resourcesPath := filepath.Join(a.getResourcePath(), "resources")

	// 先清理无效的工具路径
	if err := a.cleanInvalidToolPaths(); err != nil {
		Error("清理无效工具路径时出错: %v", err)
		// 不返回错误，继续扫描
	}

	return a.ScanToolsInPath(resourcesPath)
}

// ScanCustomDirectoryForTools 扫描自定义目录寻找工具
func (a *App) ScanCustomDirectoryForTools(customPath string) ([]ScannedTool, error) {
	// 验证路径存在
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("扫描目录不存在: %s", customPath)
	}

	return a.ScanToolsInCustomPath(customPath)
}

// CleanInvalidPaths 清理无效路径并返回清理结果
func (a *App) CleanInvalidPaths() (CleanupResult, error) {
	// 先扫描当前存在的工具，用于迁移检测
	resourcesPath := filepath.Join(a.getResourcePath(), "resources")
	scannedTools, err := a.ScanToolsInPath(resourcesPath)
	if err != nil {
		return CleanupResult{}, err
	}

	return a.cleanInvalidToolPathsWithMigration(scannedTools)
}

// ScanToolsInPath 扫描指定路径下的工具
// resources文件夹下的每个目录是分类文件夹，每个分类下的子目录是工具文件夹
func (a *App) ScanToolsInPath(scanPath string) ([]ScannedTool, error) {
	var scannedTools []ScannedTool

	// 检查扫描目录是否存在
	if _, err := os.Stat(scanPath); os.IsNotExist(err) {
		return scannedTools, fmt.Errorf("扫描目录不存在: %s", scanPath)
	}

	// 读取现有的tool.yml配置
	existingCategories, err := a.loadExistingCategories()
	if err != nil {
		return scannedTools, fmt.Errorf("读取现有配置失败: %v", err)
	}

	// 遍历resources文件夹下的分类文件夹
	categoryDirs, err := ioutil.ReadDir(scanPath)
	if err != nil {
		return scannedTools, fmt.Errorf("读取扫描目录失败: %v", err)
	}

	for _, categoryDir := range categoryDirs {
		if !categoryDir.IsDir() {
			continue
		}

		// 跳过特殊目录（Java环境目录）
		if categoryDir.Name() == "java8" || categoryDir.Name() == "java11" || categoryDir.Name() == "java17" {
			continue
		}

		categoryPath := filepath.Join(scanPath, categoryDir.Name())
		// 分类信息以分类文件夹名称为基础，如果在现有配置中存在则保持现有设置
		categoryInfo := a.getCategoryInfo(categoryDir.Name(), existingCategories)

		// 遍历分类目录下的工具文件夹
		toolDirs, err := ioutil.ReadDir(categoryPath)
		if err != nil {
			continue // 跳过无法读取的目录
		}

		for _, toolDir := range toolDirs {
			if !toolDir.IsDir() {
				continue
			}

			// 构建相对于resources的路径 - 确保始终保存相对路径格式
			toolPath := filepath.Join("resources", categoryDir.Name(), toolDir.Name())
			// 使用filepath.ToSlash确保路径分隔符统一
			toolPath = filepath.ToSlash(toolPath)

			// 扫描所有工具目录，不管是否有可执行文件
			scannedTool := ScannedTool{
				Path:          toolPath,
				Category:      categoryInfo.Name,
				PossibleFiles: []string{}, // 不再使用此字段，保留兼容性
			}
			scannedTools = append(scannedTools, scannedTool)
		}
	}

	return scannedTools, nil
}

// ScanToolsInCustomPath 扫描自定义路径下的工具（使用绝对路径）
// 支持两种目录结构：
// 1. 分类式：customPath/category1/tool1, customPath/category2/tool2
// 2. 平铺式：customPath/tool1, customPath/tool2 (统一归类为"自定义工具")
func (a *App) ScanToolsInCustomPath(scanPath string) ([]ScannedTool, error) {
	var scannedTools []ScannedTool

	// 检查扫描目录是否存在
	if _, err := os.Stat(scanPath); os.IsNotExist(err) {
		return scannedTools, fmt.Errorf("扫描目录不存在: %s", scanPath)
	}

	// 读取现有的tool.yml配置
	existingCategories, err := a.loadExistingCategories()
	if err != nil {
		return scannedTools, fmt.Errorf("读取现有配置失败: %v", err)
	}

	// 先尝试分类式扫描
	categoryScanned := false
	entries, err := ioutil.ReadDir(scanPath)
	if err != nil {
		return scannedTools, fmt.Errorf("读取扫描目录失败: %v", err)
	}

	// 检查是否是分类式结构（存在目录，且目录下还有子目录）
	for _, entry := range entries {
		if entry.IsDir() {
			categoryPath := filepath.Join(scanPath, entry.Name())
			subEntries, err := ioutil.ReadDir(categoryPath)
			if err != nil {
				continue
			}

			// 检查是否有子目录（工具目录）
			hasSubDirs := false
			for _, subEntry := range subEntries {
				if subEntry.IsDir() {
					hasSubDirs = true
					break
				}
			}

			if hasSubDirs {
				// 是分类式结构，按分类扫描
				categoryInfo := a.getCategoryInfo(entry.Name(), existingCategories)

				for _, subEntry := range subEntries {
					if subEntry.IsDir() {
						toolAbsPath := filepath.Join(categoryPath, subEntry.Name())
						scannedTool := ScannedTool{
							Path:          toolAbsPath, // 使用绝对路径
							Category:      categoryInfo.Name,
							PossibleFiles: []string{},
						}
						scannedTools = append(scannedTools, scannedTool)
					}
				}
				categoryScanned = true
			}
		}
	}

	// 如果不是分类式结构，进行平铺式扫描
	if !categoryScanned {
		defaultCategory := "自定义工具"
		categoryInfo := a.getCategoryInfo(defaultCategory, existingCategories)

		for _, entry := range entries {
			if entry.IsDir() {
				toolAbsPath := filepath.Join(scanPath, entry.Name())
				scannedTool := ScannedTool{
					Path:          toolAbsPath, // 使用绝对路径
					Category:      categoryInfo.Name,
					PossibleFiles: []string{},
				}
				scannedTools = append(scannedTools, scannedTool)
			}
		}
	}

	return scannedTools, nil
}

// CategoryInfo 分类信息结构体
type CategoryInfo struct {
	Name string
	Icon string
}

// loadExistingCategories 读取现有的分类配置
func (a *App) loadExistingCategories() (map[string]CategoryInfo, error) {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	existingCategories := make(map[string]CategoryInfo)

	// 读取现有配置
	if data, err := ioutil.ReadFile(configPath); err == nil {
		var categories Categories
		if err := yaml.Unmarshal(data, &categories); err == nil {
			// 建立目录名到分类信息的映射
			for _, category := range categories.Category {
				// 假设目录名是分类名的英文版本或简化版本
				// 这里我们反向推导可能的目录名
				possibleDirNames := a.getPossibleDirNames(category.Name)
				categoryInfo := CategoryInfo{
					Name: category.Name,
					Icon: category.Icon,
				}
				for _, dirName := range possibleDirNames {
					existingCategories[dirName] = categoryInfo
				}
			}
		}
	}

	return existingCategories, nil
}

// getPossibleDirNames 根据分类名获取可能的目录名
func (a *App) getPossibleDirNames(categoryName string) []string {
	// 基本的映射规则，可以根据需要扩展
	mapping := map[string][]string{
		"信息收集":         {"info", "information", "recon"},
		"渗透利器":         {"pentest", "penetration", "exploit"},
		"Webshell管理工具": {"webshell", "shell", "backdoor"},
		"框架利用工具":       {"framework", "comprehensive", "exploit"},
		"数据库利用":        {"databases", "database", "db"},
		"代理":           {"proxy", "proxies"}, // 修复：代理 -> proxy
		"代理工具":         {"proxy", "proxies"},
		"内网工具":         {"intranet", "Intranet"}, // 添加内网工具映射
		"其他":           {"other", "misc", "miscellaneous"},
	}

	if dirNames, exists := mapping[categoryName]; exists {
		return dirNames
	}

	// 如果没有映射，尝试简单的转换
	simplified := strings.ToLower(strings.ReplaceAll(categoryName, " ", ""))
	return []string{simplified, categoryName}
}

// getCategoryInfo 获取分类信息，优先使用现有配置
func (a *App) getCategoryInfo(dirName string, existingCategories map[string]CategoryInfo) CategoryInfo {
	if existingInfo, exists := existingCategories[dirName]; exists {
		return existingInfo
	}
	// 如果没有现有映射，返回默认分类信息
	return CategoryInfo{
		Name: dirName,
		Icon: "", // 默认图标为空
	}
}

// getRelativePath 获取相对于资源目录的路径
func (a *App) getRelativePath(scanPath string) string {
	resourcesPath := filepath.Join(a.getResourcePath(), "resources")
	if strings.HasPrefix(scanPath, resourcesPath) {
		// 如果是resources子目录，返回相对于resources目录的路径
		relPath, _ := filepath.Rel(resourcesPath, scanPath)
		// 如果相对路径为空（即scanPath就是resourcesPath），返回"resources"
		if relPath == "." {
			return "resources"
		}
		// 否则返回"resources/相对路径"
		return filepath.Join("resources", relPath)
	}
	// 如果是外部路径，返回完整路径
	return scanPath
}

// formatCategoryName 格式化分类名称（保留用于兼容性）
func (a *App) formatCategoryName(dirName string) string {
	// 现在主要由getCategoryName处理，这里保留简单逻辑
	return dirName
}

// formatToolName 格式化工具名称
func (a *App) formatToolName(dirName string) string {
	// 移除版本号和特殊字符，保留主要名称
	name := strings.ReplaceAll(dirName, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// 首字母大写
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}

// scanExecutableFiles 扫描目录下的可执行文件（递归扫描子目录）
func (a *App) scanExecutableFiles(toolDir string) ([]string, error) {
	var executableFiles []string

	// 使用filepath.Walk递归扫描目录
	err := filepath.Walk(toolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略无法访问的文件/目录，继续扫描
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 获取相对于工具目录的路径
		relPath, err := filepath.Rel(toolDir, path)
		if err != nil {
			return nil // 忽略路径错误，继续扫描
		}

		// 检查文件是否为可执行文件
		if a.isExecutableFile(info.Name(), info) {
			// 使用相对路径，这样用户可以看到文件在子目录中
			executableFiles = append(executableFiles, relPath)
		}

		return nil
	})

	if err != nil {
		return executableFiles, err
	}

	return executableFiles, nil
}

// analyzeToolDirectory 分析工具目录内容，决定如何添加工具
func (a *App) analyzeToolDirectory(toolDir string) (toolType string, fileName string, command string) {
	// 读取工具目录内容
	files, err := ioutil.ReadDir(toolDir)
	if err != nil {
		return "openterm", "", ""
	}

	// 如果目录为空，返回openterm
	if len(files) == 0 {
		return "openterm", "", ""
	}

	// 查找jar文件和exe文件
	var jarFiles []string
	var exeFiles []string

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		lowerName := strings.ToLower(file.Name())

		// 检查jar文件
		if strings.HasSuffix(lowerName, ".jar") {
			jarFiles = append(jarFiles, file.Name())
		} else if strings.HasSuffix(lowerName, ".exe") {
			exeFiles = append(exeFiles, file.Name())
		}
	}

	// 优先级：jar > exe
	if len(jarFiles) > 0 {
		// 如果有jar文件，使用默认Java版本打开
		defaultJava := a.getDefaultJavaVersion()
		return defaultJava, jarFiles[0], "-jar"
	}

	if len(exeFiles) > 0 {
		// 如果有exe文件，使用Open打开
		return "Open", exeFiles[0], ""
	}

	// 如果只有子目录或其他文件，使用openterm
	return "openterm", "", ""
}

// isExecutableFile 判断文件是否为可执行文件
func (a *App) isExecutableFile(fileName string, fileInfo os.FileInfo) bool {
	// 检查常见的可执行文件扩展名
	lowerName := strings.ToLower(fileName)
	if strings.HasSuffix(lowerName, ".jar") ||
		strings.HasSuffix(lowerName, ".exe") ||
		strings.HasSuffix(lowerName, ".bat") ||
		strings.HasSuffix(lowerName, ".cmd") {
		return true
	}

	// 在Windows上，.bat/.cmd已经覆盖
	// 其他文件类型（非Windows）检查执行权限
	if runtime.GOOS != "windows" && !fileInfo.IsDir() {
		// 检查是否有执行权限
		if fileInfo.Mode()&0111 != 0 {
			// 进一步检查是否为二进制文件（排除脚本文件）
			return a.isBinaryExecutable(fileName)
		}
	}

	return false
}

// isBinaryExecutable 判断是否为二进制可执行文件
func (a *App) isBinaryExecutable(fileName string) bool {
	// 简单检查：无扩展名且不是常见的文本文件
	ext := filepath.Ext(fileName)
	if ext == "" {
		// 排除常见的配置文件和文档
		lowerName := strings.ToLower(fileName)
		excludePatterns := []string{"readme", "license", "changelog", "makefile", "dockerfile", ".gitignore", ".gitattributes"}
		for _, pattern := range excludePatterns {
			if strings.Contains(lowerName, pattern) {
				return false
			}
		}
		return true
	}
	return false
}

// setExecutionType 根据文件类型设置执行方式
// jar文件默认使用Java打开，exe文件使用Open打开，其他的使用openterm打开
func (a *App) setExecutionType(tool *Tool, fileName string) {
	lowerFileName := strings.ToLower(fileName)

	if strings.HasSuffix(lowerFileName, ".jar") {
		// jar文件默认用设置中配置的默认Java版本打开
		tool.Value = a.getDefaultJavaVersion()
		tool.Command = "-jar"
	} else if strings.HasSuffix(lowerFileName, ".exe") {
		// exe文件默认用Open打开
		tool.Value = "Open"
		tool.Command = ""
	} else {
		// 其他文件（包括.bat、.cmd、.sh、.py、无扩展名的二进制文件等）都使用openterm打开
		tool.Value = "openterm"
		tool.Command = ""
	}
}

// selectBestExecutableFile 从多个可执行文件中选择最适合的默认文件
func (a *App) selectBestExecutableFile(files []string) string {
	// 优先级：jar > exe > 无扩展名的二进制 > 其他
	priorities := map[string]int{
		".jar": 1,
		".exe": 2,
		"":     3, // 无扩展名
	}

	bestFile := ""
	bestPriority := 999

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if priority, exists := priorities[ext]; exists {
			if priority < bestPriority {
				bestPriority = priority
				bestFile = file
			}
		} else if bestPriority > 5 {
			// 如果没有找到高优先级文件，选择第一个
			bestFile = file
			bestPriority = 5
		}
	}

	return bestFile
}

// SelectDirectory 选择目录（用于前端文件夹选择器）
func (a *App) SelectDirectory() (string, error) {
	// 使用Wails运行时打开文件夹选择对话框
	selectedPath, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择要扫描的文件夹",
	})

	if err != nil {
		return "", fmt.Errorf("打开文件夹选择对话框失败: %v", err)
	}

	return selectedPath, nil
}

// SelectFile 选择文件
func (a *App) SelectFile() (string, error) {
	// 使用Wails运行时打开文件选择对话框
	selectedFile, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择文件",
		// 不设置Filters，这样可以选择任意文件包括二进制文件
	})

	if err != nil {
		return "", fmt.Errorf("打开文件选择对话框失败: %v", err)
	}

	return selectedFile, nil
}

// OpenGitHubPage 在默认浏览器中打开GitHub页面
func (a *App) OpenGitHubPage() error {
	githubURL := "https://github.com/Diger-Young/Spear"

	// Windows下使用 cmd /C start 打开默认浏览器
	cmd := exec.Command("cmd", "/C", "start", githubURL)
	return cmd.Start()
}

// SelectJavaPath 选择Java路径（选择具体的Java可执行文件）
func (a *App) SelectJavaPath() (string, error) {
	// 直接选择Java可执行文件
	selectedFile, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择Java可执行文件",
		// 不设置Filters，这样可以选择任意文件包括二进制的java可执行文件
	})

	if err != nil {
		return "", fmt.Errorf("选择Java路径失败: %v", err)
	}

	return selectedFile, nil
}

// GetNewToolsFromScanned 获取真正的新工具（过滤掉已存在的）
func (a *App) GetNewToolsFromScanned(tools []ScannedTool) ([]ScannedTool, error) {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")
	var categories Categories
	var config Config

	// 读取现有配置
	if data, err := ioutil.ReadFile(configPath); err == nil {
		yaml.Unmarshal(data, &categories)
		yaml.Unmarshal(data, &config)
	}

	// 获取现有工具的路径作为唯一标识，避免重复添加同一个工具目录
	// 对于有FileName的工具，需要同时考虑Path和Path+FileName的组合
	existingToolPaths := make(map[string]bool)
	existingToolPathFileNames := make(map[string]bool) // Path+FileName的组合
	for _, category := range categories.Category {
		for _, tool := range category.Tool {
			// 使用路径作为唯一标识
			existingToolPaths[tool.Path] = true
			// 如果有FileName，也记录Path+FileName的组合
			if tool.FileName != "" {
				combinedPath := filepath.Join(tool.Path, tool.FileName)
				existingToolPathFileNames[filepath.ToSlash(combinedPath)] = true
			}
		}
	}

	// 过滤出真正的新工具
	var newTools []ScannedTool
	for _, scannedTool := range tools {
		// 检查这个扫描到的工具路径是否已经存在
		// 需要检查：1) 路径本身 2) 路径+FileName的组合
		scannedPath := filepath.ToSlash(scannedTool.Path)
		isExisting := existingToolPaths[scannedPath]

		// 如果路径不存在，还需要检查是否有Path+FileName的组合匹配
		if !isExisting {
			// 检查是否有工具使用这个路径作为Path，FileName为子项名
			for existingPath := range existingToolPaths {
				if strings.HasPrefix(existingPath, scannedPath+"/") {
					// 检查是否有工具使用这个路径
					relPath := strings.TrimPrefix(existingPath, scannedPath+"/")
					if relPath != "" && !strings.Contains(relPath, "/") {
						// 可能是FileName
						isExisting = true
						break
					}
				}
			}
		}

		if !isExisting {
			newTools = append(newTools, scannedTool)
		}
	}

	return newTools, nil
}

// AutoAddScannedTools 自动添加扫描到的工具
func (a *App) AutoAddScannedTools(tools []ScannedTool) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	var categories Categories
	var config Config
	if data, err := ioutil.ReadFile(configPath); err == nil {
		yaml.Unmarshal(data, &categories)
		yaml.Unmarshal(data, &config)
	}

	// 建立现有分类的映射，保留图标信息
	existingCategoryMap := make(map[string]Category)
	for _, category := range categories.Category {
		existingCategoryMap[category.Name] = category
	}

	// 获取现有工具的路径作为唯一标识，避免重复添加同一个工具目录
	// 对于有FileName的工具，需要同时考虑Path和Path+FileName的组合
	existingToolPaths := make(map[string]bool)
	existingToolPathFileNames := make(map[string]bool) // Path+FileName的组合
	for _, category := range categories.Category {
		for _, tool := range category.Tool {
			// 使用路径作为唯一标识
			existingToolPaths[tool.Path] = true
			// 如果有FileName，也记录Path+FileName的组合
			if tool.FileName != "" {
				combinedPath := filepath.Join(tool.Path, tool.FileName)
				existingToolPathFileNames[filepath.ToSlash(combinedPath)] = true
			}
		}
	}

	// 添加新发现的工具
	for _, scannedTool := range tools {
		// 检查这个扫描到的工具路径是否已经存在
		scannedPath := filepath.ToSlash(scannedTool.Path)
		isExisting := existingToolPaths[scannedPath]

		// 如果路径不存在，还需要检查是否有Path+FileName的组合匹配
		if !isExisting {
			// 检查是否有工具使用这个路径作为Path，FileName为子项名
			for existingPath := range existingToolPaths {
				if strings.HasPrefix(existingPath, scannedPath+"/") {
					relPath := strings.TrimPrefix(existingPath, scannedPath+"/")
					if relPath != "" && !strings.Contains(relPath, "/") {
						isExisting = true
						break
					}
				}
			}
		}

		if isExisting {
			Debug("跳过已存在的工具: 路径: %s", scannedTool.Path)
			continue // 跳过已存在的工具
		}

		// 从路径中提取工具名称（使用文件夹名）
		pathParts := strings.Split(scannedTool.Path, "/")
		toolName := pathParts[len(pathParts)-1] // 取最后一部分作为工具名
		if toolName == "" {
			toolName = "Unknown Tool"
		}

		// 分析工具目录内容，决定如何添加工具
		basePath := a.getResourcePath()
		fullToolPath := filepath.Join(basePath, scannedTool.Path)
		toolType, fileName, command := a.analyzeToolDirectory(fullToolPath)

		// 创建新工具，使用智能分析的结果
		newTool := Tool{
			Name:        a.formatToolName(toolName),
			Path:        scannedTool.Path,
			FileName:    fileName,
			Value:       toolType,
			Command:     command,
			Optional:    "",
			Description: fmt.Sprintf("扫描发现的工具路径: %s", scannedTool.Path),
		}

		// 查找或创建分类
		categoryFound := false
		for i, category := range categories.Category {
			if category.Name == scannedTool.Category {
				categories.Category[i].Tool = append(categories.Category[i].Tool, newTool)
				categoryFound = true
				break
			}
		}

		if !categoryFound {
			// 创建新分类，如果有现有分类信息则保留图标
			newCategory := Category{
				Name: scannedTool.Category,
				Tool: []Tool{newTool},
			}
			// 如果在现有分类映射中找到，保留图标信息
			if existingCategory, exists := existingCategoryMap[scannedTool.Category]; exists {
				newCategory.Icon = existingCategory.Icon
			}
			categories.Category = append(categories.Category, newCategory)
		}
	}

	// 保存配置
	return a.saveCategoriesToFile(categories, config)
}

// saveCategoriesToFile 保存分类配置到文件
func (a *App) saveCategoriesToFile(categories Categories, config Config) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 确保配置文件所在目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %v", err)
	}

	// 创建备份
	backupPath := configPath + ".backup"
	if _, err := os.Stat(configPath); err == nil {
		if err := os.Rename(configPath, backupPath); err != nil {
			Error("创建备份失败: %v", err)
		} else {
			Info("已创建配置备份: %s", backupPath)
		}
	}

	// 检查Java配置是否为空，如果为空则尝试保持原有配置
	javaConfig := config.JavaPaths
	if javaConfig.Java8 == "" && javaConfig.Java11 == "" && javaConfig.Java17 == "" {
		// 尝试从备份中读取原有配置
		backupPath := configPath + ".backup"
		if _, err := os.Stat(backupPath); err == nil {
			if backupData, err := ioutil.ReadFile(backupPath); err == nil {
				var backupConfig Config
				if err := yaml.Unmarshal(backupData, &backupConfig); err == nil {
					// 如果备份中有Java配置，使用备份的配置
					if backupConfig.JavaPaths.Java8 != "" || backupConfig.JavaPaths.Java11 != "" || backupConfig.JavaPaths.Java17 != "" {
						javaConfig = backupConfig.JavaPaths
						Info("从备份中恢复Java配置")
					}
				}
			}
		}

		// 如果备份也没有，使用默认的Java配置
		if javaConfig.Java8 == "" && javaConfig.Java11 == "" && javaConfig.Java17 == "" {
			javaConfig = JavaConfig{
				Java8:  "resources/java8/bin/java",
				Java11: "resources/java11/bin/java",
				Java17: "resources/java17/bin/java",
			}
			Info("使用默认Java配置")
		}
	}

	// 构建完整的配置对象
	fullConfig := Config{
		JavaPaths:  javaConfig,
		Categories: categories.Category,
	}

	// 序列化完整配置
	data, err := yaml.Marshal(fullConfig)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	// 添加注释头部，注意不要与序列化的内容重复
	content := fmt.Sprintf(`# Java配置
# 自定义Java路径配置，如果留空将使用系统默认Java
%s`, string(data))

	// 使用原子写入：先写入临时文件，然后重命名
	tempPath := configPath + ".tmp"

	// 写入临时文件
	if err := ioutil.WriteFile(tempPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入临时配置文件失败: %v", err)
	}

	// 原子重命名，替换原文件
	if err := os.Rename(tempPath, configPath); err != nil {
		// 如果重命名失败，清理临时文件
		os.Remove(tempPath)
		return fmt.Errorf("替换配置文件失败: %v", err)
	}

	Info("配置文件已更新: %s", configPath)

	// 验证写入的文件是否正确
	if err := a.validateConfigFile(configPath); err != nil {
		Warn("配置文件验证失败: %v", err)
		// 尝试恢复备份
		backupPath := configPath + ".backup"
		if _, err := os.Stat(backupPath); err == nil {
			if err := os.Rename(backupPath, configPath); err == nil {
				Info("已从备份恢复配置文件")
			}
		}
		return fmt.Errorf("配置文件验证失败，请检查: %v", err)
	}

	// 发送更新成功事件
	wailsRuntime.EventsEmit(a.ctx, "tools-scanned", true)
	return nil
}

// validateConfigFile 验证配置文件的完整性
func (a *App) validateConfigFile(configPath string) error {
	// 读取文件内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 尝试解析YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("YAML解析失败: %v", err)
	}

	// 检查是否有重复的Categories键
	content := string(data)
	categoriesCount := strings.Count(content, "Categories:")
	if categoriesCount > 1 {
		return fmt.Errorf("发现重复的Categories键，数量: %d", categoriesCount)
	}

	// 检查JavaPaths是否存在
	javaPaths := strings.Count(content, "javapath:")
	if javaPaths != 1 {
		return fmt.Errorf("JavaPaths键异常，数量: %d", javaPaths)
	}

	Info("配置文件验证通过，Categories数量: %d, 工具总数: %d",
		len(config.Categories),
		func() int {
			total := 0
			for _, cat := range config.Categories {
				total += len(cat.Tool)
			}
			return total
		}())

	return nil
}

// UpdateCategoryName 更新分类名称
func (a *App) UpdateCategoryName(oldName, newName string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并更新分类名称
	found := false
	for i, category := range categories.Category {
		if category.Name == oldName {
			categories.Category[i].Name = newName
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("分类 '%s' 不存在", oldName)
	}

	// 保存配置
	return a.saveCategoriesToFile(categories, config)
}

// UpdateCategoriesOrder 更新分类顺序
func (a *App) UpdateCategoriesOrder(orderedCategories []Category) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	var config Config
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 更新分类顺序
	config.Categories = orderedCategories

	// 构建Categories结构
	categories := Categories{
		Category: orderedCategories,
	}

	// 保存配置
	return a.saveCategoriesToFile(categories, config)
}

// UpdateCategoryIcon 更新分类图标
func (a *App) UpdateCategoryIcon(categoryName, icon string) error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取原始YAML内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析Categories和Config
	var categories Categories
	var config Config
	if err := yaml.Unmarshal(data, &categories); err != nil {
		return fmt.Errorf("解析Categories失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 查找并更新分类图标
	found := false
	for i, category := range categories.Category {
		if category.Name == categoryName {
			categories.Category[i].Icon = icon
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("分类 '%s' 不存在", categoryName)
	}

	// 保存配置
	return a.saveCategoriesToFile(categories, config)
}

// FileInfo 文件信息结构体
type FileInfo struct {
	Name         string `json:"name"`
	IsDir        bool   `json:"isDir"`
	Size         int64  `json:"size"`
	ModTime      string `json:"modTime"`
	Path         string `json:"path"`
	Extension    string `json:"extension"`
	IsExecutable bool   `json:"isExecutable"`
}

// BrowseDirectory 浏览指定目录（支持相对路径和绝对路径）
func (a *App) BrowseDirectory(pathInput string) ([]FileInfo, error) {
	var fileInfos []FileInfo

	// 调试信息
	Debug("BrowseDirectory 调用，输入路径: %s", pathInput)

	var fullPath string

	// 判断是绝对路径还是相对路径
	if filepath.IsAbs(pathInput) {
		// 绝对路径，直接使用
		fullPath = pathInput
		Debug("使用绝对路径: %s", fullPath)
	} else {
		// 相对路径，构建基于resources的完整路径
		basePath := a.getResourcePath()

		if pathInput == "" || pathInput == "/" {
			// 浏览resources根目录
			fullPath = filepath.Join(basePath, "resources")
			Debug("浏览根目录，完整路径: %s", fullPath)
		} else {
			// 浏览指定子目录
			fullPath = filepath.Join(basePath, "resources", pathInput)
			Debug("浏览子目录，完整路径: %s", fullPath)
		}
	}

	// 检查路径是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fileInfos, fmt.Errorf("目录不存在: %s (完整路径: %s)", pathInput, fullPath)
	}

	// 读取目录内容
	files, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return fileInfos, fmt.Errorf("读取目录失败: %v", err)
	}

	// 构建文件信息列表
	for _, file := range files {
		fileName := file.Name()

		// 跳过隐藏文件
		if strings.HasPrefix(fileName, ".") {
			continue
		}

		fileExt := strings.ToLower(filepath.Ext(fileName))
		isExecutable := a.isExecutableFile(fileName, file)

		var filePath string
		if pathInput == "" || pathInput == "/" {
			filePath = fileName
		} else {
			filePath = filepath.ToSlash(filepath.Join(pathInput, fileName))
		}

		fileInfo := FileInfo{
			Name:         fileName,
			IsDir:        file.IsDir(),
			Size:         file.Size(),
			ModTime:      file.ModTime().Format("2006-01-02 15:04:05"),
			Path:         filePath,
			Extension:    fileExt,
			IsExecutable: isExecutable,
		}

		fileInfos = append(fileInfos, fileInfo)
	}

	return fileInfos, nil
}

// GetFileTypes 获取预定义的执行方式列表
func (a *App) GetFileTypes() []map[string]string {
	return []map[string]string{
		{"value": "Java8", "label": "Java 8", "description": "使用Java 8运行JAR文件"},
		{"value": "Java11", "label": "Java 11", "description": "使用Java 11运行JAR文件"},
		{"value": "Java17", "label": "Java 17", "description": "使用Java 17运行JAR文件"},
		{"value": "Open", "label": "系统打开", "description": "使用系统默认方式打开文件"},
		{"value": "openterm", "label": "终端打开", "description": "在终端中打开目录"},
		{"value": "python", "label": "Python", "description": "使用Python运行脚本"},
		{"value": "custom", "label": "自定义命令", "description": "使用自定义系统命令"},
	}
}

// GetToolDirectory 获取工具目录的文件列表（用于编辑工具时选择文件）
func (a *App) GetToolDirectory(toolPath string) ([]FileInfo, error) {
	// 调试信息：记录原始路径
	Debug("GetToolDirectory 调用，原始路径: %s", toolPath)

	// 使用统一的路径清理方法
	cleanedPath := a.cleanToolPath(toolPath)

	// 移除resources前缀，因为BrowseDirectory会自动添加
	cleanPath := strings.TrimPrefix(cleanedPath, "resources/")

	// 如果路径为空，返回错误
	if cleanPath == "" {
		return nil, fmt.Errorf("工具路径不能为空")
	}

	Debug("最终使用的相对路径: %s", cleanPath)

	return a.BrowseDirectory(cleanPath)
}

// DebugAllToolPaths 调试方法：打印所有工具的路径配置
func (a *App) DebugAllToolPaths() error {
	categories, err := a.GetCategories()
	if err != nil {
		return err
	}

	Debug("=== 调试：所有工具路径配置 ===")
	for _, category := range categories.Category {
		Debug("分类: %s", category.Name)
		for _, tool := range category.Tool {
			Debug("  工具: %s, 路径: %s", tool.Name, tool.Path)
		}
	}
	Debug("=== 调试结束 ===")
	return nil
}

// CleanupToolPaths 清理和修复工具路径
func (a *App) CleanupToolPaths() error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析Config失败: %v", err)
	}

	// 从config中获取categories，确保包含所有信息包括图标
	categories := Categories{
		Category: config.Categories,
	}

	// 清理每个工具的路径
	pathsFixed := 0
	for i, category := range categories.Category {
		for j, tool := range category.Tool {
			originalPath := tool.Path
			cleanedPath := a.cleanToolPath(tool.Path)

			if originalPath != cleanedPath {
				Debug("修复工具路径: %s -> %s", originalPath, cleanedPath)
				categories.Category[i].Tool[j].Path = cleanedPath
				pathsFixed++
			}
		}
	}

	// 如果有路径被修复，保存配置
	if pathsFixed > 0 {
		Info("总共修复了 %d 个工具路径", pathsFixed)
		return a.saveCategoriesToFile(categories, config)
	}

	Info("没有发现需要修复的路径")
	return nil
}

// cleanToolPath 清理工具路径
func (a *App) cleanToolPath(path string) string {
	originalPath := path

	// 如果是空字符串，直接返回
	if path == "" {
		return path
	}

	// 如果是URL，直接返回原样
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	// 如果是绝对路径，直接返回原样（不需要清理）
	if filepath.IsAbs(path) {
		return path
	}

	// 统一将路径分隔符转换为正斜杠，方便处理
	path = strings.ReplaceAll(path, "\\", "/")

	// 只对相对路径进行清理
	// 1. 处理包含 "Contents/Resources/" 的错误拼接路径（兼容 Mac .app 包）
	contentsResources := "Contents/Resources"
	if strings.Contains(path, contentsResources) {
		// 找到最后一个 Contents/Resources 的位置
		lastIndex := strings.LastIndex(path, contentsResources)
		if lastIndex != -1 {
			// 提取后面的部分
			suffix := path[lastIndex+len(contentsResources)+1:]
			// 如果后面还有resources/，则移除第一个
			if strings.HasPrefix(suffix, "resources/") {
				path = suffix
			} else if suffix != "" {
				path = "resources/" + suffix
			} else {
				path = "resources"
			}
		}
	}

	// 2. 移除开头的多余斜杠
	path = strings.TrimPrefix(path, "/")

	// 3. 确保相对路径以resources/开头（但只对非空路径）
	if path != "" && !strings.HasPrefix(path, "resources/") && !strings.HasPrefix(path, "resources") {
		path = "resources/" + path
	}

	// 4. 移除重复的resources前缀
	for strings.Contains(path, "resources/resources/") {
		path = strings.ReplaceAll(path, "resources/resources/", "resources/")
	}

	// 5. 清理路径中的重复斜杠
	path = filepath.Clean(path)

	// 6. 转换为统一的正斜杠格式，确保跨平台兼容性
	path = filepath.ToSlash(path)

	// 调试输出
	if originalPath != path {
		Debug("路径清理: %s -> %s", originalPath, path)
	}

	return path
}

// RepairConfigFile 修复损坏的配置文件
func (a *App) RepairConfigFile() error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 首先验证当前配置文件是否正常
	if err := a.validateConfigFile(configPath); err == nil {
		// 配置文件正常，无需修复
		return nil
	}

	Info("检测到配置文件异常，开始修复...")
	backupPath := configPath + ".backup"

	// 检查是否有备份文件
	if _, err := os.Stat(backupPath); err == nil {
		Info("发现备份文件: %s", backupPath)

		// 验证备份文件
		if err := a.validateConfigFile(backupPath); err == nil {
			Info("备份文件验证通过，开始恢复...")

			// 删除损坏的文件
			if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
				Error("删除损坏文件失败: %v", err)
			}

			// 恢复备份
			if err := os.Rename(backupPath, configPath); err != nil {
				return fmt.Errorf("恢复备份失败: %v", err)
			}

			Info("配置文件修复成功！")
			return nil
		} else {
			Error("备份文件也已损坏: %v", err)
		}
	}

	// 如果没有可用备份，创建默认配置
	Info("没有可用备份，创建默认配置...")
	defaultConfig := Config{
		JavaPaths: JavaConfig{
			Java8:  "",
			Java11: "",
			Java17: "",
		},
		Categories: []Category{},
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("序列化默认配置失败: %v", err)
	}

	content := fmt.Sprintf(`# Java配置
# 自定义Java路径配置，如果留空将使用系统默认Java
%s`, string(data))

	if err := ioutil.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入默认配置失败: %v", err)
	}

	Info("已创建默认配置文件")
	return nil
}

// CleanupDuplicateTools 清理重复的工具
func (a *App) CleanupDuplicateTools() error {
	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	var config Config
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 从config中获取categories，确保包含所有信息包括图标
	categories := Categories{
		Category: config.Categories,
	}

	Info("开始清理重复工具...")

	// 记录已处理的工具路径和对应的分类映射
	processedPaths := make(map[string]string) // path -> 最佳分类名
	duplicatesFound := 0

	// 第一轮：找出最佳分类（优先选择中文分类名）
	for _, category := range categories.Category {
		for _, tool := range category.Tool {
			if existingCategory, exists := processedPaths[tool.Path]; exists {
				// 发现重复工具
				duplicatesFound++
				Debug("发现重复工具: %s", tool.Path)
				Debug("  已存在分类: %s", existingCategory)
				Debug("  当前分类: %s", category.Name)

				// 选择更好的分类名（中文优先，或者更长的名称）
				if a.isBetterCategoryName(category.Name, existingCategory) {
					processedPaths[tool.Path] = category.Name
					Debug("  选择分类: %s", category.Name)
				} else {
					Debug("  保持分类: %s", existingCategory)
				}
			} else {
				processedPaths[tool.Path] = category.Name
			}
		}
	}

	if duplicatesFound == 0 {
		Info("没有发现重复工具")
		return nil
	}

	Info("发现 %d 个重复工具，开始合并...", duplicatesFound)

	// 第二轮：重建分类，合并重复工具
	newCategories := []Category{}
	categoryMap := make(map[string]*Category) // 分类名 -> 分类对象

	for _, category := range categories.Category {
		for _, tool := range category.Tool {
			bestCategoryName := processedPaths[tool.Path]

			// 找到或创建目标分类
			if targetCategory, exists := categoryMap[bestCategoryName]; exists {
				// 检查工具是否已经存在于目标分类中
				toolExists := false
				for _, existingTool := range targetCategory.Tool {
					if existingTool.Path == tool.Path {
						toolExists = true
						// 如果新工具有文件名而现有工具没有，则更新
						if existingTool.FileName == "" && tool.FileName != "" {
							existingTool.FileName = tool.FileName
							existingTool.Value = tool.Value
							existingTool.Command = tool.Command
							Debug("更新工具文件名: %s -> %s", tool.Path, tool.FileName)
						}
						break
					}
				}

				if !toolExists {
					targetCategory.Tool = append(targetCategory.Tool, tool)
				}
			} else {
				// 创建新分类
				newCategory := Category{
					Name: bestCategoryName,
					Icon: category.Icon, // 保留原分类的图标
					Tool: []Tool{tool},
				}
				newCategories = append(newCategories, newCategory)
				categoryMap[bestCategoryName] = &newCategories[len(newCategories)-1]
			}
		}
	}

	// 更新配置
	categories.Category = newCategories

	// 保存配置
	if err := a.saveCategoriesToFile(categories, config); err != nil {
		return fmt.Errorf("保存配置失败: %v", err)
	}

	Info("重复工具清理完成，合并了 %d 个重复工具", duplicatesFound)
	return nil
}

// isBetterCategoryName 判断哪个分类名更好
func (a *App) isBetterCategoryName(name1, name2 string) bool {
	// 中文分类名优先
	if a.isChinese(name1) && !a.isChinese(name2) {
		return true
	}
	if !a.isChinese(name1) && a.isChinese(name2) {
		return false
	}

	// 如果都是中文或都是英文，选择更长的（更具描述性的）
	return len(name1) > len(name2)
}

// isChinese 判断字符串是否包含中文字符
func (a *App) isChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// CleanupResult 清理结果统计
type CleanupResult struct {
	InvalidToolsCount      int      `json:"invalidToolsCount"`      // 清理的无效工具数量
	InvalidCategoriesCount int      `json:"invalidCategoriesCount"` // 清理的无效分类数量
	CleanedNotes           int      `json:"cleanedNotes"`           // 清理的笔记文件数量
	MigratedNotes          int      `json:"migratedNotes"`          // 迁移的笔记文件数量
	InvalidToolNames       []string `json:"invalidToolNames"`       // 被清理的工具名称列表
	MigratedToolNames      []string `json:"migratedToolNames"`      // 被迁移的工具名称列表
}

// cleanInvalidToolPaths 清理配置中无效的工具路径
func (a *App) cleanInvalidToolPaths() error {
	_, err := a.cleanInvalidToolPathsWithResult()
	return err
}

// cleanInvalidToolPathsWithResult 清理配置中无效的工具路径并返回详细结果
func (a *App) cleanInvalidToolPathsWithResult() (CleanupResult, error) {
	result := CleanupResult{
		InvalidToolNames: []string{},
	}

	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	var config Config
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return result, fmt.Errorf("解析Config失败: %v", err)
	}

	// 从config中获取categories，确保包含所有信息包括图标
	categories := Categories{
		Category: config.Categories,
	}

	basePath := a.getResourcePath()
	cleanedCategories := Categories{}
	originalCategoryCount := len(categories.Category)

	// 遍历所有分类和工具，检查路径有效性
	for _, category := range categories.Category {
		cleanedCategory := Category{
			Name: category.Name,
			Icon: category.Icon, // 保留分类图标
			Tool: []Tool{},
		}

		for _, tool := range category.Tool {
			// 对于Browser类型的工具，如果路径是URL，跳过文件系统检查
			if tool.Value == "Browser" && (strings.HasPrefix(tool.Path, "http://") || strings.HasPrefix(tool.Path, "https://")) {
				// URL类型的工具，直接保留
				cleanedCategory.Tool = append(cleanedCategory.Tool, tool)
				continue
			}

			// 检查工具路径是否存在
			// 对于有FileName的工具，需要检查Path+FileName的组合路径
			var fullToolPath string
			if filepath.IsAbs(tool.Path) {
				// 绝对路径，直接使用（用户手动添加的工具可能使用绝对路径）
				if tool.FileName != "" {
					fullToolPath = filepath.Join(tool.Path, tool.FileName)
				} else {
					fullToolPath = tool.Path
				}
			} else {
				// 相对路径，基于basePath构建
				if tool.FileName != "" {
					// 如果有FileName，检查Path+FileName的组合
					fullToolPath = filepath.Join(basePath, tool.Path, tool.FileName)
				} else {
					// 如果没有FileName，只检查Path
					fullToolPath = filepath.Join(basePath, tool.Path)
				}
			}

			// 规范化路径（处理 .. 和 . 等）
			fullToolPath = filepath.Clean(fullToolPath)

			// 检查路径是否存在
			pathExists := false
			if _, err := os.Stat(fullToolPath); err == nil {
				pathExists = true
			} else {
				// 添加调试信息
				Debug("检查路径失败 - Path: %s, FileName: %s, 完整路径: %s, 错误: %v",
					tool.Path, tool.FileName, fullToolPath, err)
			}

			if !pathExists {
				// 路径不存在，标记为无效
				Debug("发现无效工具路径: %s (工具: %s, FileName: %s, 完整路径: %s)", tool.Path, tool.Name, tool.FileName, fullToolPath)
				result.InvalidToolsCount++
				result.InvalidToolNames = append(result.InvalidToolNames, tool.Name)

				// 删除对应的笔记文件
				if a.cleanToolNote(tool) {
					result.CleanedNotes++
				}
				continue
			}

			// 路径存在，保留这个工具
			cleanedCategory.Tool = append(cleanedCategory.Tool, tool)
		}

		// 只保留有工具的分类
		if len(cleanedCategory.Tool) > 0 {
			cleanedCategories.Category = append(cleanedCategories.Category, cleanedCategory)
		} else if len(category.Tool) > 0 {
			// 如果原来有工具但现在没有了，说明整个分类的工具都无效了
			Info("分类 '%s' 的所有工具都无效，已删除该分类", category.Name)
		}
	}

	// 计算被删除的分类数量
	result.InvalidCategoriesCount = originalCategoryCount - len(cleanedCategories.Category)

	// 如果有无效工具被清理，保存更新的配置
	if result.InvalidToolsCount > 0 {
		if err := a.saveCategoriesToFile(cleanedCategories, config); err != nil {
			return result, fmt.Errorf("保存清理后的配置失败: %v", err)
		}
		Info("已清理 %d 个无效工具路径，%d 个无效分类，%d 个笔记文件",
			result.InvalidToolsCount, result.InvalidCategoriesCount, result.CleanedNotes)
	}

	return result, nil
}

// cleanToolNote 清理工具对应的笔记文件，返回是否成功清理
func (a *App) cleanToolNote(tool Tool) bool {
	// 生成工具ID（与前端逻辑保持一致）
	toolPath := tool.Path
	if toolPath != "" {
		pathParts := strings.Split(toolPath, "/")
		if len(pathParts) > 0 {
			// 使用工具目录名作为ID
			toolDirName := pathParts[len(pathParts)-1]
			toolId := strings.ReplaceAll(toolDirName, " ", "_")
			toolId = strings.ReplaceAll(toolId, "-", "_")

			// 尝试删除对应的笔记文件
			notesDir := filepath.Join(a.getResourcePath(), "notes")
			noteFile := filepath.Join(notesDir, fmt.Sprintf("%s.md", toolId))

			if _, err := os.Stat(noteFile); err == nil {
				if err := os.Remove(noteFile); err == nil {
					Debug("已清理无效工具的笔记文件: %s", noteFile)
					return true
				}
			}
		}
	}
	return false
}

// cleanInvalidToolPathsWithMigration 清理配置中无效的工具路径，支持智能迁移检测
func (a *App) cleanInvalidToolPathsWithMigration(scannedTools []ScannedTool) (CleanupResult, error) {
	result := CleanupResult{
		InvalidToolNames:  []string{},
		MigratedToolNames: []string{},
	}

	configPath := filepath.Join(a.getDataDir(), "tool.yml")

	// 读取现有配置
	var config Config
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return result, fmt.Errorf("解析Config失败: %v", err)
	}

	// 从config中获取categories，确保包含所有信息包括图标
	categories := Categories{
		Category: config.Categories,
	}

	// 创建扫描到的工具目录名映射，用于迁移检测
	scannedToolDirs := make(map[string]ScannedTool)
	for _, scannedTool := range scannedTools {
		pathParts := strings.Split(scannedTool.Path, "/")
		if len(pathParts) > 0 {
			toolDirName := pathParts[len(pathParts)-1]
			scannedToolDirs[toolDirName] = scannedTool
		}
	}

	basePath := a.getResourcePath()
	cleanedCategories := Categories{}
	originalCategoryCount := len(categories.Category)

	// 遍历所有分类和工具，检查路径有效性
	for _, category := range categories.Category {
		cleanedCategory := Category{
			Name: category.Name,
			Icon: category.Icon, // 保留分类图标
			Tool: []Tool{},
		}

		for _, tool := range category.Tool {
			// 对于Browser类型的工具，如果路径是URL，跳过文件系统检查
			if tool.Value == "Browser" && (strings.HasPrefix(tool.Path, "http://") || strings.HasPrefix(tool.Path, "https://")) {
				// URL类型的工具，直接保留
				cleanedCategory.Tool = append(cleanedCategory.Tool, tool)
				continue
			}

			// 检查工具路径是否存在
			// 对于有FileName的工具，需要检查Path+FileName的组合路径
			var fullToolPath string
			if filepath.IsAbs(tool.Path) {
				// 绝对路径，直接使用（用户手动添加的工具可能使用绝对路径）
				if tool.FileName != "" {
					fullToolPath = filepath.Join(tool.Path, tool.FileName)
				} else {
					fullToolPath = tool.Path
				}
			} else {
				// 相对路径，基于basePath构建
				if tool.FileName != "" {
					// 如果有FileName，检查Path+FileName的组合
					fullToolPath = filepath.Join(basePath, tool.Path, tool.FileName)
				} else {
					// 如果没有FileName，只检查Path
					fullToolPath = filepath.Join(basePath, tool.Path)
				}
			}

			// 规范化路径（处理 .. 和 . 等）
			fullToolPath = filepath.Clean(fullToolPath)

			// 检查路径是否存在
			pathExists := false
			if _, err := os.Stat(fullToolPath); err == nil {
				pathExists = true
			} else {
				// 添加调试信息
				Debug("检查路径失败 - Path: %s, FileName: %s, 完整路径: %s, 错误: %v",
					tool.Path, tool.FileName, fullToolPath, err)
			}

			if !pathExists {
				// 对于绝对路径的工具，如果路径不存在，直接标记为无效（不进行迁移检测）
				// 因为绝对路径通常是用户手动添加的，不应该被迁移
				if filepath.IsAbs(tool.Path) {
					Debug("发现无效工具路径（绝对路径）: %s (工具: %s, FileName: %s, 完整路径: %s)", tool.Path, tool.Name, tool.FileName, fullToolPath)
					result.InvalidToolsCount++
					result.InvalidToolNames = append(result.InvalidToolNames, tool.Name)

					// 删除对应的笔记文件
					if a.cleanToolNote(tool) {
						result.CleanedNotes++
					}
					continue
				}

				// 对于相对路径，检查是否有迁移的可能
				pathParts := strings.Split(tool.Path, "/")
				if len(pathParts) > 0 {
					toolDirName := pathParts[len(pathParts)-1]

					// 检查是否有相同工具目录名的新工具
					if newScannedTool, exists := scannedToolDirs[toolDirName]; exists {
						// 发现可能的迁移，迁移笔记而不是删除
						Info("检测到工具迁移: %s (%s -> %s)", tool.Name, tool.Path, newScannedTool.Path)

						if a.migrateToolNote(tool, toolDirName) {
							result.MigratedNotes++
							result.MigratedToolNames = append(result.MigratedToolNames, tool.Name)
							Debug("已迁移工具笔记: %s", tool.Name)
						}

						// 标记为无效工具（配置会被清理，但笔记已迁移）
						result.InvalidToolsCount++
						result.InvalidToolNames = append(result.InvalidToolNames, tool.Name)
						continue
					}
				}

				// 没有找到迁移目标，按原逻辑处理
				Debug("发现无效工具路径: %s (工具: %s, FileName: %s)", tool.Path, tool.Name, tool.FileName)
				result.InvalidToolsCount++
				result.InvalidToolNames = append(result.InvalidToolNames, tool.Name)

				// 删除对应的笔记文件
				if a.cleanToolNote(tool) {
					result.CleanedNotes++
				}
				continue
			}

			// 路径存在，保留这个工具
			cleanedCategory.Tool = append(cleanedCategory.Tool, tool)
		}

		// 只保留有工具的分类
		if len(cleanedCategory.Tool) > 0 {
			cleanedCategories.Category = append(cleanedCategories.Category, cleanedCategory)
		} else if len(category.Tool) > 0 {
			// 如果原来有工具但现在没有了，说明整个分类的工具都无效了
			Info("分类 '%s' 的所有工具都无效，已删除该分类", category.Name)
		}
	}

	// 计算被删除的分类数量
	result.InvalidCategoriesCount = originalCategoryCount - len(cleanedCategories.Category)

	// 如果有无效工具被清理，保存更新的配置
	if result.InvalidToolsCount > 0 {
		if err := a.saveCategoriesToFile(cleanedCategories, config); err != nil {
			return result, fmt.Errorf("保存清理后的配置失败: %v", err)
		}
		Info("已清理 %d 个无效工具路径，%d 个无效分类，%d 个笔记文件，迁移 %d 个笔记文件",
			result.InvalidToolsCount, result.InvalidCategoriesCount, result.CleanedNotes, result.MigratedNotes)
	}

	return result, nil
}

// migrateToolNote 迁移工具笔记（实际上是保持不变，因为新旧工具使用相同的目录名ID）
func (a *App) migrateToolNote(tool Tool, toolDirName string) bool {
	// 由于新旧工具的目录名相同，笔记ID也相同，所以实际上不需要做任何操作
	// 只需要检查笔记文件是否存在
	toolId := strings.ReplaceAll(toolDirName, " ", "_")
	toolId = strings.ReplaceAll(toolId, "-", "_")

	notesDir := filepath.Join(a.getResourcePath(), "notes")
	noteFile := filepath.Join(notesDir, fmt.Sprintf("%s.md", toolId))

	if _, err := os.Stat(noteFile); err == nil {
		// 笔记文件存在，迁移成功（实际上是保持原状）
		return true
	}

	return false
}

// iconCacheEntry 缓存条目（包含文件修改时间，避免图标过期）
type iconCacheEntry struct {
	Data    string `json:"data"`    // base64 编码的图标
	ModTime int64  `json:"modTime"` // 文件修改时间（用于检测图标更新）
}

// getIconCachePath 获取图标缓存文件路径
func (a *App) getIconCachePath() string {
	return filepath.Join(a.getDataDir(), "icon_cache.json")
}

// loadIconCache 加载图标缓存
func (a *App) loadIconCache() map[string]iconCacheEntry {
	cache := make(map[string]iconCacheEntry)
	cachePath := a.getIconCachePath()
	data, err := ioutil.ReadFile(cachePath)
	if err != nil {
		return cache
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		Debug("图标缓存解析失败: %v", err)
		return cache
	}
	return cache
}

// saveIconCache 保存图标缓存到文件
func (a *App) saveIconCache(cache map[string]iconCacheEntry) error {
	cachePath := a.getIconCachePath()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(cachePath, data, 0644)
}

// GetToolIcon 获取工具文件的图标，返回 base64 编码的图片数据 URI
func (a *App) GetToolIcon(toolPath, fileName string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", nil
	}
	// 获取完整的文件路径
	var fullPath string
	if filepath.IsAbs(fileName) {
		fullPath = filepath.Clean(fileName)
	} else if filepath.IsAbs(toolPath) {
		fullPath = filepath.Clean(filepath.Join(toolPath, fileName))
	} else {
		abs, err := a.GetToolAbsolutePath(toolPath, fileName)
		if err != nil {
			return "", nil
		}
		fullPath = abs
	}
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", nil
	}

	// 规范化路径作为缓存 key（统一小写+正斜杠）
	normalizedPath := strings.ToLower(strings.ReplaceAll(fullPath, "\\", "/"))
	modTime := fileInfo.ModTime().Unix()

	// 加载并检查缓存
	iconCache := a.loadIconCache()
	if entry, ok := iconCache[normalizedPath]; ok && entry.ModTime == modTime {
		Debug("使用缓存图标: %s", fullPath)
		return entry.Data, nil
	}

	// 使用 PowerShell 通过 .NET ExtractAssociatedIcon 提取文件图标并编码为 base64 PNG
	psCmd := fmt.Sprintf(`Add-Type -AssemblyName System.Drawing;
try {
	$icon = [System.Drawing.Icon]::ExtractAssociatedIcon('%s');
	if ($icon -ne $null) {
		$bmp = $icon.ToBitmap();
		$ms = New-Object System.IO.MemoryStream;
		$bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png);
		$bmp.Dispose();
		$icon.Dispose();
		[System.Convert]::ToBase64String($ms.ToArray())
	}
} catch {}`, strings.ReplaceAll(fullPath, "'", "''"))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return "", nil
	}
	b64 := strings.TrimSpace(string(output))
	if b64 == "" {
		return "", nil
	}
	data := "data:image/png;base64," + b64

	// 保存到缓存
	iconCache[normalizedPath] = iconCacheEntry{
		Data:    data,
		ModTime: modTime,
	}
	if err := a.saveIconCache(iconCache); err != nil {
		Debug("保存图标缓存失败: %v", err)
	}

	return data, nil
}

// GetToolIcons 批量获取工具图标
func (a *App) GetToolIcons(tools []Tool) (map[string]string, error) {
	result := make(map[string]string)
	for _, tool := range tools {
		icon, _ := a.GetToolIcon(tool.Path, tool.FileName)
		if icon != "" {
			result[tool.Name] = icon
		}
	}
	return result, nil
}
