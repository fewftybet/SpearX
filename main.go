package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
//go:embed build/appicon.png
var assets embed.FS

// 加载应用图标
// 优先使用 build/windows/icon.ico（Windows 原生 ICO 多尺寸格式）
// 如果不存在，回退到 build/appicon.png
func loadIcon() []byte {
	// 优先尝试 ICO 格式
	if data, err := os.ReadFile("build/windows/icon.ico"); err == nil {
		return data
	}
	// 回退到 PNG
	if data, err := os.ReadFile("build/appicon.png"); err == nil {
		return data
	}
	return nil
}

func main() {
	// 加载应用图标
	icon := loadIcon()

	// 初始化日志系统
	if err := InitLogger(); err != nil {
		Error("初始化日志失败: %v", err)
	}
	defer CloseLogger()

	// 捕获 panic 并写入 error.log
	defer func() {
		if r := recover(); r != nil {
			LogErrorWithContext("MAIN_PANIC", fmt.Errorf("%v", r))
			log.Fatal(r)
		}
	}()

	// 创建一个新的应用实例
	app := NewApp()

	// 创建应用配置
	err := wails.Run(&options.App{
		Title:  "SpearX",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		// 启用文件拖拽功能（让 OnFileDrop 能接收到完整路径）
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: false,
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
		// Windows 设置
		// 注意：Wails Windows 图标通过构建时嵌入 build/windows/icon.ico 实现，
		// 不通过运行时设置 Icon 字段（windows.Options 没有 Icon 字段）
		Windows: &windows.Options{},
		// Mac 设置
		Mac: &mac.Options{
			Appearance:           mac.NSAppearanceNameDarkAqua, // 固定使用深色模式
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			TitleBar:             mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "SpearX",
				Message: "A modern Windows tool manager\n\nDeveloped by Diger_Young\n© 2026 Diger_Young\n\nVersion 2.0.0",
				Icon:    icon, // 使用嵌入的图标
			},
		},
	})

	if err != nil {
		LogErrorWithContext("WAILS_RUN", err)
		log.Fatal(err)
	}
}

func (a *App) Select(selectFolder bool) (string, error) {
	var dialog string
	var err error

	options := runtime.OpenDialogOptions{
		Title: "选择工具",
	}

	if selectFolder {
		dialog, err = runtime.OpenDirectoryDialog(a.ctx, options)
	} else {
		options.Filters = []runtime.FileFilter{
			{
				DisplayName: "所有文件 (*)",
				Pattern:     "*",
			},
		}
		dialog, err = runtime.OpenFileDialog(a.ctx, options)
	}

	if err != nil {
		return "", err
	}

	return dialog, nil
}
