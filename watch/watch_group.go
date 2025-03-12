package watch

import (
	"fmt"
	"sync"

	"github.com/kmlixh/consulTool"
	"github.com/kmlixh/consulTool/errors"
)

// WatchGroup 监控组
type WatchGroup struct {
	watches map[string]*consulTool.Watch
	mu      sync.RWMutex
}

// NewWatchGroup 创建新的监控组
func NewWatchGroup() *WatchGroup {
	return &WatchGroup{
		watches: make(map[string]*consulTool.Watch),
	}
}

// Add 添加监控
func (wg *WatchGroup) Add(name string, watch *consulTool.Watch) error {
	if name == "" {
		return errors.NewError(errors.ErrCodeValidation, "watch name cannot be empty", nil)
	}
	if watch == nil {
		return errors.NewError(errors.ErrCodeValidation, "watch cannot be nil", nil)
	}

	wg.mu.Lock()
	defer wg.mu.Unlock()

	if _, exists := wg.watches[name]; exists {
		return errors.NewError(errors.ErrCodeValidation, fmt.Sprintf("watch with name %s already exists", name), nil)
	}

	wg.watches[name] = watch
	return nil
}

// Remove 移除监控
func (wg *WatchGroup) Remove(name string) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	watch, exists := wg.watches[name]
	if !exists {
		return errors.NewError(errors.ErrCodeValidation, fmt.Sprintf("watch with name %s does not exist", name), nil)
	}

	// 停止监控
	if err := watch.StopWatchService(name); err != nil {
		return errors.NewError(errors.ErrCodeUnknown, fmt.Sprintf("failed to stop watch %s", name), err)
	}

	delete(wg.watches, name)
	return nil
}

// Get 获取监控
func (wg *WatchGroup) Get(name string) (*consulTool.Watch, bool) {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	watch, exists := wg.watches[name]
	return watch, exists
}

// StopAll 停止所有监控
func (wg *WatchGroup) StopAll() error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	var lastErr error
	for name, watch := range wg.watches {
		if err := watch.StopWatchService(name); err != nil {
			lastErr = errors.NewError(errors.ErrCodeUnknown, fmt.Sprintf("failed to stop watch %s", name), err)
		}
	}

	// 清空map
	wg.watches = make(map[string]*consulTool.Watch)
	return lastErr
}

// List 列出所有监控名称
func (wg *WatchGroup) List() []string {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	names := make([]string, 0, len(wg.watches))
	for name := range wg.watches {
		names = append(names, name)
	}
	return names
}

// Count 获取监控数量
func (wg *WatchGroup) Count() int {
	wg.mu.RLock()
	defer wg.mu.RUnlock()
	return len(wg.watches)
}
