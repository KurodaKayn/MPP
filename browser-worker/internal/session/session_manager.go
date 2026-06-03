package session

import "sync"

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*WorkerSession
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*WorkerSession),
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
