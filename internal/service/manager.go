package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ServiceManager struct {
	statusPath string
	pidPath    string
}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		statusPath: filepath.Join(os.TempDir(), "serialhub_status.json"),
		pidPath:    filepath.Join(os.TempDir(), "serialhub.pid"),
	}
}

func (sm *ServiceManager) WriteStatus(mcpPort int) error {
	status := map[string]any{
		"pid":     os.Getpid(),
		"mcpPort": mcpPort,
		"status":  "running",
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}
	if err := os.WriteFile(sm.statusPath, data, 0644); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	return nil
}

func (sm *ServiceManager) ClearStatus() error {
	if err := os.Remove(sm.statusPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清除状态文件失败: %w", err)
	}
	return nil
}

func (sm *ServiceManager) ReadStatus() (map[string]any, error) {
	data, err := os.ReadFile(sm.statusPath)
	if err != nil {
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}
	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %w", err)
	}
	return status, nil
}
