package session

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	workerPoolSizeEnv        = "BROWSER_WORKER_POOL_SIZE"
	defaultWorkerPoolSize    = 4
	unlimitedWorkerPoolLimit = 0
)

type Manager struct {
	mu           sync.RWMutex
	sessions     map[string]*WorkerSession
	reservations int
	limit        int
}

func NewManager() *Manager {
	return NewManagerWithLimit(WorkerPoolSizeFromEnv())
}

func NewManagerWithLimit(limit int) *Manager {
	if limit < 0 {
		limit = unlimitedWorkerPoolLimit
	}
	return &Manager{
		sessions: make(map[string]*WorkerSession),
		limit:    limit,
	}
}

func WorkerPoolSizeFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(workerPoolSizeEnv))
	if raw == "" {
		return defaultWorkerPoolSize
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return defaultWorkerPoolSize
	}
	return value
}

func (sm *Manager) TryReserve() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.limit > 0 && len(sm.sessions)+sm.reservations >= sm.limit {
		return false
	}
	sm.reservations++
	return true
}

func (sm *Manager) ReleaseReservation() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.reservations > 0 {
		sm.reservations--
	}
}

func (sm *Manager) Get(ref string) (*WorkerSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[ref]
	return session, ok
}

func (sm *Manager) Put(session *WorkerSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[session.ID] = session
}

func (sm *Manager) Remove(ref string) (*WorkerSession, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	session, ok := sm.sessions[ref]
	if ok {
		delete(sm.sessions, ref)
	}
	return session, ok
}

func (sm *Manager) List() []*WorkerSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*WorkerSession, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}
