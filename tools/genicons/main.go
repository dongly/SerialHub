package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
)

// PNG 图标数据（16x16 RGBA）
var pngIcons = map[string]string{
	"tray-idle.png":      "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAT0lEQVQ4y2NgGAWjYBSMAjqA/wSKcYH/BIrxgv8EilGB/wQKAQb/ESgGFvxHoBhY8B+BQmDBfwSKQYH/BIpRgf8ESnGB/wSKcYH/BMoBAMNmIcNfHpGPAAAAAElFTkSuQmCC",
	"tray-connected.png": "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAWklEQVQ4y2NgGAWjYBSMAjqA/wSKcYH/BIrxgv8EilGB/wQKAQb/ESgGFvxHoBhY8B+BQmDBfwSKQYH/BIpRgf8EimDBfwQKgYH/BIoBhf8EigEF/wSKAYX/BMoBAMV9IcNfEpGPAAAAAElFTkSuQmCC",
	"tray-error.png":     "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAUUlEQVQ4y2NgGAWjYBSMAjqA/wSKcYH/BIrxgv8EilGB/wQKAQb/ESgGFvxHoBhY8B+BQmDBfwSKQYH/BMoBBf8EigEF/wTKAQX/BMoBAHkYIcNfE5GPAAAAAElFTkSuQmCC",
}

// createICO 从 PNG 数据创建 ICO 文件
func createICO(pngData []byte) []byte {
	// ICO 文件结构：
	// ICONDIR (6 bytes): reserved(2), type(2=1 for icon), count(2)
	// ICONDIRENTRY (16 bytes per image): width, height, colors, reserved, planes, bpp, size, offset
	// Image data (PNG)

	width := byte(16)  // 16x16
	height := byte(16) // 16x16
	colors := byte(0)  // 0 for >= 256 colors
	reserved := byte(0)
	planes := uint16(1)
	bpp := uint16(32) // 32-bit (RGBA)
	size := uint32(len(pngData))
	offset := uint32(22) // 6 + 16 = 22

	// ICONDIR header
	ico := []byte{
		0, 0, // reserved
		1, 0, // type: 1 = icon
		1, 0, // count: 1 image
	}

	// ICONDIRENTRY
	ico = append(ico,
		width, height, colors, reserved,
		byte(planes), byte(planes>>8),
		byte(bpp), byte(bpp>>8),
		byte(size), byte(size>>8), byte(size>>16), byte(size>>24),
		byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
	)

	// PNG image data
	ico = append(ico, pngData...)

	return ico
}

func main() {
	dir := "pkg/tray/assets"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	// 生成 PNG 文件（供非 Windows 平台使用）
	for filename, base64Data := range pngIcons {
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			panic(err)
		}

		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, data, 0644); err != nil {
			panic(err)
		}
		println("Written PNG:", path, len(data), "bytes")
	}

	// 生成 ICO 文件（供 Windows 平台使用）
	for pngFilename, base64Data := range pngIcons {
		pngData, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			panic(err)
		}

		icoData := createICO(pngData)
		icoFilename := pngFilename[:len(pngFilename)-4] + ".ico"
		path := filepath.Join(dir, icoFilename)

		if err := os.WriteFile(path, icoData, 0644); err != nil {
			panic(err)
		}
		println("Written ICO:", path, len(icoData), "bytes")
	}
}
