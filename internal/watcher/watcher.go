// Package watcher watches config files and triggers hot reloads.
// It supports cross-platform fsnotify event handling.
package watcher

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"gopkg.in/yaml.v3"

	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// Watcher manages file watching for configuration files.
type Watcher struct {
	configPath        string
	config            *config.Config
	clientsMutex      sync.RWMutex
	configReloadMu    sync.Mutex
	configReloadTimer *time.Timer
	reloadCallback    func(*config.Config)
	watcher           *fsnotify.Watcher
	lastConfigHash    string
	authQueue         chan<- AuthUpdate
	currentAuths      map[string]*coreauth.Auth
	runtimeAuths      map[string]*coreauth.Auth
	dispatchMu        sync.Mutex
	dispatchCond      *sync.Cond
	pendingUpdates    map[string]AuthUpdate
	pendingOrder      []string
	dispatchCancel    context.CancelFunc
	oldConfigYaml     []byte
}

// AuthUpdateAction represents the type of change detected in auth sources.
type AuthUpdateAction string

const (
	AuthUpdateActionAdd    AuthUpdateAction = "add"
	AuthUpdateActionModify AuthUpdateAction = "modify"
	AuthUpdateActionDelete AuthUpdateAction = "delete"
)

// AuthUpdate describes an incremental change to auth configuration.
type AuthUpdate struct {
	Action AuthUpdateAction
	ID     string
	Auth   *coreauth.Auth
}

const (
	configReloadDebounce = 150 * time.Millisecond
)

// NewWatcher creates a new file watcher instance
func NewWatcher(configPath string, reloadCallback func(*config.Config)) (*Watcher, error) {
	watcher, errNewWatcher := fsnotify.NewWatcher()
	if errNewWatcher != nil {
		return nil, errNewWatcher
	}
	w := &Watcher{
		configPath:     configPath,
		reloadCallback: reloadCallback,
		watcher:        watcher,
	}
	w.dispatchCond = sync.NewCond(&w.dispatchMu)
	return w, nil
}

// Start begins watching the configuration file.
func (w *Watcher) Start(ctx context.Context) error {
	return w.start(ctx)
}

// Stop stops the file watcher
func (w *Watcher) Stop() error {
	w.stopDispatch()
	w.stopConfigReloadTimer()
	return w.watcher.Close()
}

// SetConfig updates the current configuration
func (w *Watcher) SetConfig(cfg *config.Config) {
	w.clientsMutex.Lock()
	defer w.clientsMutex.Unlock()
	w.config = cfg
	w.oldConfigYaml, _ = yaml.Marshal(cfg)
}

// SetAuthUpdateQueue sets the queue used to emit auth updates.
func (w *Watcher) SetAuthUpdateQueue(queue chan<- AuthUpdate) {
	w.setAuthUpdateQueue(queue)
}

// DispatchRuntimeAuthUpdate allows external runtime providers (e.g., websocket-driven auths)
// to push auth updates through the same queue used by file/config watchers.
// Returns true if the update was enqueued; false if no queue is configured.
func (w *Watcher) DispatchRuntimeAuthUpdate(update AuthUpdate) bool {
	return w.dispatchRuntimeAuthUpdate(update)
}

// SnapshotCoreAuths converts current clients snapshot into core auth entries.
func (w *Watcher) SnapshotCoreAuths() []*coreauth.Auth {
	w.clientsMutex.RLock()
	cfg := w.config
	w.clientsMutex.RUnlock()
	return snapshotCoreAuths(cfg)
}
