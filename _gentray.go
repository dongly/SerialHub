package main
import ("os"; "fmt")
func main() {
    content := "// Package tray 提供系统托盘管理功能。\npackage tray\n"
    os.WriteFile("pkg/tray/tray.go", []byte(content), 0644)
    fmt.Println("Done")
}
