package sessionstate

import (
	"context"
	"log"
	"time"

	"github.com/kurodakayn/mpp-browser-worker/internal/cdp"
	"github.com/kurodakayn/mpp-browser-worker/internal/cookies"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func StartLoop(ctx context.Context, workerSession *session.WorkerSession) context.CancelFunc {
	loopCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(session.HeartbeatRefreshInterval)
		defer ticker.Stop()

		for {
			if _, err := DetectAndSave(loopCtx, workerSession); err != nil {
				if loopCtx.Err() != nil {
					return
				}
				state := transientReadState(workerSession, err)
				if saveErr := workerSession.StateStore.SaveLiveSession(loopCtx, workerSession, state); saveErr != nil && loopCtx.Err() == nil {
					log.Printf("browser session state save failed worker_session_ref=%s err=%v", workerSession.ID, saveErr)
				}
			}
			if err := workerSession.StateStore.RefreshHeartbeat(loopCtx, workerSession); err != nil && loopCtx.Err() == nil {
				log.Printf("browser session heartbeat refresh failed worker_session_ref=%s err=%v", workerSession.ID, err)
			}

			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

func transientReadState(workerSession *session.WorkerSession, err error) session.WorkerSessionState {
	status := workerSession.Status
	if status == "" || status == "failed" {
		status = "ready"
	}
	return session.WorkerSessionState{
		WorkerSessionRef: workerSession.ID,
		Status:           status,
		LoginDetected:    status == "login_detected",
		Message:          "Temporarily unable to read browser state: " + err.Error(),
		ExpiresAt:        workerSession.ExpiresAt,
	}
}

func DetectAndSave(ctx context.Context, workerSession *session.WorkerSession) (session.WorkerSessionState, error) {
	currentURL, browserCookies, _, err := cdp.Snapshot(ctx, workerSession, false)
	if err != nil {
		return session.WorkerSessionState{}, err
	}

	ok, missing := cookies.ValidateRequired(browserCookies, workerSession.RequiredCookies)
	status := "ready"
	message := "Waiting for required login cookies"
	if ok {
		status = "login_detected"
		message = "Login detected successfully"
	}

	state := session.WorkerSessionState{
		WorkerSessionRef: workerSession.ID,
		Status:           status,
		CurrentURL:       currentURL,
		LoginDetected:    ok,
		MissingCookies:   missing,
		Message:          message,
		ExpiresAt:        workerSession.ExpiresAt,
	}
	workerSession.Status = status
	if err := workerSession.StateStore.SaveLiveSession(ctx, workerSession, state); err != nil {
		return session.WorkerSessionState{}, err
	}
	return state, nil
}
