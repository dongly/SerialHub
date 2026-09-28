// Package instance 提供实例级身份标识与单实例互斥：
//   - 单实例 lock：以操作系统文件锁（Unix flock / Windows LockFileEx）为
//     唯一所有权依据，内核保证持锁进程退出（含崩溃/被杀）时自动释放；
//     lock 文件内容（JSON 元数据）仅用于发现与诊断。
//   - 侧别标识：本进程运行在 Windows 还是 WSL（串口列表 side 字段用）。
//
// 协议要点：
//   - lock 文件永不删除：删除会引发 inode 竞态（A 持锁旧 inode、B 新建
//     文件锁新 inode → 双主）。stale 场景（持锁进程已死）锁已被内核释放，
//     下一次 Acquire 直接在同一文件上重新加锁并覆盖内容。
//   - Read 只做发现：锁不可获取 → 活主存在（返回其元数据）；锁可获取 →
//     无活主（返回 nil）。不做 HTTP 验活、不删除文件。
package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrActive 表示已有存活的主实例持有 lock。
var ErrActive = errors.New("已有运行中的主实例")

// LockInfo 是 instance.lock 的内容，描述当前主实例。
type LockInfo struct {
	PID       int    `json:"pid"`        // 主实例进程号（展示与诊断用，非所有权凭据）
	Port      int    `json:"port"`       // HTTP 服务端口（MCP/Web/终端共用）
	Host      string `json:"host"`       // 监听地址（通常 127.0.0.1）
	StartedAt string `json:"started_at"` // 启动时间（RFC3339，展示用）
}

// URL 返回主实例的 HTTP 基址（监听通配地址转为回环，IPv6 正确加方括号）。
func (l LockInfo) URL() string {
	host := l.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(l.Port))
}

// IsWSL 判断当前是否运行在 WSL 环境中。
func IsWSL() bool {
	// Windows 构建永不运行在 WSL 内（GOOS=linux 才可能）。
	// 仅靠 WSL_DISTRO_NAME 判断会把从 WSL interop 启动的 Windows 进程
	// （继承该环境变量）误判为 WSL 侧（历史 bug，12a613c 修复）。
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	// 旧版 WSL 无 WSL_DISTRO_NAME，读内核版本描述
	if b, err := os.ReadFile("/proc/version"); err == nil {
		return strings.Contains(strings.ToLower(string(b)), "microsoft")
	}
	return false
}

// LocalSide 返回本实例所在侧标识（windows/wsl）。
func LocalSide() string {
	if IsWSL() {
		return "wsl"
	}
	return "windows"
}

// lockFile 是本进程持有的 lock 文件句柄；持锁期间必须保持引用
// （fd 被 GC finalizer 关闭会释放锁），Release 时关闭。
var lockFile *os.File
var lockMu sync.Mutex

// lockPathOverride 供测试注入 lock 路径（跨平台隔离真实路径）。
var lockPathOverride string

// lockPath 返回 lock 文件路径：Linux/macOS 用用户配置目录（与配置文件
// 同目录），Windows 用可执行文件同目录——两处均与 config.toml 同级，
// 卸载时随配置一起清理。
func lockPath() string {
	if lockPathOverride != "" {
		return lockPathOverride
	}
	if runtime.GOOS != "windows" {
		if base := xdgConfigDir(); base != "" {
			return filepath.Join(base, "serialhub", "instance.lock")
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "instance.lock")
}

// xdgConfigDir 解析 XDG 用户配置基目录：XDG_CONFIG_HOME（绝对路径）优先，
// 否则 HOME/.config；均不可得返回空串。
func xdgConfigDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

// WaitForInfo 有界等待 lock 元数据可读：主实例持锁后、写入元数据完成前
// 存在短暂窗口，此时 Read 返回零值 LockInfo（Port=0）。返回首个可读元数据，
// 超时或无活主返回 nil。
func WaitForInfo(timeout time.Duration) *LockInfo {
	deadline := time.Now().Add(timeout)
	for {
		if info := Read(); info != nil && info.Port > 0 {
			return info
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// healthProbeTimeout 是单次就绪探测请求的超时。
var healthProbeTimeout = 1500 * time.Millisecond

// WaitReady 轮询 info 指向实例的 /health，直到确认为本产品实例（service=serialhub）
// 或超过 timeout。主实例先持锁写元数据、后启动 HTTP，二者之间存在时间差；
// stdio 代理前用它等待主实例 HTTP 就绪。等待就绪不改变所有权：返回 false 时
// 调用方应报错，绝不可据此接管主实例的锁。
func WaitReady(info LockInfo, timeout time.Duration) bool {
	if info.Port <= 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	for {
		if healthOK(info.URL() + "/health") {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// healthOK 请求指定 /health 并校验身份（有界读取 + JSON 解析，不依赖
// 单次 Body.Read 返回完整响应）。
func healthOK(url string) bool {
	client := &http.Client{Timeout: healthProbeTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		Service string `json:"service"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return false
	}
	return body.Service == "serialhub"
}

// Read 发现现有主实例：lock 文件上的 OS 锁被占用 → 活主存在，返回其
// 元数据（元数据暂不可读时返回零值 LockInfo，Port 供调用方判可用性）；
// 锁可获取（持锁进程已退出）或文件不存在 → 返回 nil。
// 不做 HTTP 验活、不修改文件——活性由内核文件锁保证。
func Read() *LockInfo {
	path := lockPath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil // 文件不存在或不可读：无活主
	}
	if err := tryLock(f); err != nil {
		if !lockBusy(err) {
			f.Close()
			return nil
		}
		// 锁被占用：活主存在，尽力读元数据
		info := readInfo(f)
		f.Close()
		if info == nil {
			return &LockInfo{} // 活主但元数据暂不可读（可能正在写入）
		}
		return info
	}
	// 锁可获取：无活主（持锁进程已退出）。释放刚取得的锁后返回 nil；
	// 不删文件（防 inode 竞态），内容由下一次 Acquire 覆盖。
	f.Close()
	return nil
}

// Acquire 原子获取单实例锁：以 OS 文件锁互斥（两个进程同时启动时仅一个
// 成功，与 HTTP 是否就绪无关）。成功后写入本进程元数据并保持锁直到
// Release/进程退出；已有活主时返回其元数据与 ErrActive（元数据不可读
// 时返回零值 LockInfo）。
func Acquire(host string, port int) (LockInfo, error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	if lockFile != nil {
		return *readInfoOrEmpty(lockFile), ErrActive
	}
	path := lockPath()
	if path == "" {
		return LockInfo{}, errors.New("无法确定 lock 文件路径")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return LockInfo{}, fmt.Errorf("创建配置目录失败: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return LockInfo{}, fmt.Errorf("打开 lock 文件失败: %w", err)
	}
	if err := tryLock(f); err != nil {
		if !lockBusy(err) {
			f.Close()
			return LockInfo{}, fmt.Errorf("锁定实例文件失败: %w", err)
		}
		// 已有活主：尽力读元数据供诊断展示
		info := LockInfo{}
		if existing := readInfo(f); existing != nil {
			info = *existing
		}
		f.Close()
		return info, ErrActive
	}
	// 取锁成功：写入本进程元数据
	info := LockInfo{
		PID:       os.Getpid(),
		Port:      port,
		Host:      host,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		f.Close()
		return LockInfo{}, fmt.Errorf("序列化 lock 元数据失败: %w", err)
	}
	if err := writeInfo(f, append(b, '\n')); err != nil {
		f.Close()
		return LockInfo{}, fmt.Errorf("写入 lock 元数据失败: %w", err)
	}
	lockFile = f
	return info, nil
}

// Release 释放本进程持有的单实例锁（幂等）。仅关闭 fd（内核释放锁），
// 不删文件——文件常驻供下一次发现/互斥使用。
func Release() {
	lockMu.Lock()
	defer lockMu.Unlock()
	if lockFile != nil {
		lockFile.Close()
		lockFile = nil
	}
}

func readInfoOrEmpty(f *os.File) *LockInfo {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return &LockInfo{}
	}
	if info := readInfo(f); info != nil {
		return info
	}
	return &LockInfo{}
}

// readInfo 尽力读取 lock 文件中的元数据（可能读到持锁进程写入中的半截
// 内容，解析失败返回 nil 而非报错——元数据仅诊断用）。
func readInfo(f *os.File) *LockInfo {
	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil || len(data) == 0 {
		return nil
	}
	var info LockInfo
	if json.Unmarshal(data, &info) != nil {
		return nil
	}
	return &info
}

// writeInfo 在已持锁的文件上覆写元数据（truncate + 从头写入 + sync）。
func writeInfo(f *os.File, data []byte) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		return err
	}
	return f.Sync()
}
