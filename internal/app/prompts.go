package app

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"ssh-first/internal/secrets"
	sshpkg "ssh-first/internal/ssh"
)

const promptTimeout = 2 * time.Minute

type hostKeyResponse struct {
	accept bool
}

type passwordResponse struct {
	password string
	remember bool
	ok       bool
}

type passphraseResponse struct {
	passphrase string
	remember   bool
	ok         bool
}

type keyboardInteractiveResponse struct {
	answers []string
	ok      bool
}

// pendingPrompts tracks in-flight frontend dialogs keyed by a request ID, so
// the corresponding Respond* method (called once the user answers) can wake
// up the Go goroutine blocked inside the SSH handshake.
type pendingPrompts struct {
	mu          sync.Mutex
	hostKey     map[string]chan hostKeyResponse
	password    map[string]chan passwordResponse
	passphrase  map[string]chan passphraseResponse
	interactive map[string]chan keyboardInteractiveResponse
}

func newPendingPrompts() *pendingPrompts {
	return &pendingPrompts{
		hostKey:     make(map[string]chan hostKeyResponse),
		password:    make(map[string]chan passwordResponse),
		passphrase:  make(map[string]chan passphraseResponse),
		interactive: make(map[string]chan keyboardInteractiveResponse),
	}
}

func (a *App) approveHostKey(req sshpkg.HostKeyDecisionRequest) (bool, error) {
	id := uuid.NewString()
	ch := make(chan hostKeyResponse, 1)

	a.pending.mu.Lock()
	a.pending.hostKey[id] = ch
	a.pending.mu.Unlock()
	defer func() {
		a.pending.mu.Lock()
		delete(a.pending.hostKey, id)
		a.pending.mu.Unlock()
	}()

	status := "unknown"
	if req.Status == sshpkg.HostKeyChanged {
		status = "changed"
	}
	a.emit("hostkey:request", HostKeyPromptEvent{
		RequestID:            id,
		Hostname:             req.Hostname,
		Algorithm:            req.Algorithm,
		Fingerprint:          req.Fingerprint,
		Status:               status,
		PreviousFingerprints: req.PreviousFingerprints,
	})

	select {
	case resp := <-ch:
		return resp.accept, nil
	case <-time.After(promptTimeout):
		return false, fmt.Errorf("host key confirmation timed out")
	}
}

// RespondHostKey answers a pending "hostkey:request" prompt.
func (a *App) RespondHostKey(requestID string, accept bool) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.hostKey[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending host key request %q", requestID)
	}
	ch <- hostKeyResponse{accept: accept}
	return nil
}

func (a *App) providePassword(auth resolvedAuth, allowRemember bool) sshpkg.PasswordProvider {
	return func(user, hostname string) (string, bool, error) {
		if allowRemember {
			if pw, err := auth.getPassword(); err == nil {
				return pw, true, nil
			}
		}

		id := uuid.NewString()
		ch := make(chan passwordResponse, 1)
		a.pending.mu.Lock()
		a.pending.password[id] = ch
		a.pending.mu.Unlock()
		defer func() {
			a.pending.mu.Lock()
			delete(a.pending.password, id)
			a.pending.mu.Unlock()
		}()

		a.emit("password:request", PasswordPromptEvent{
			RequestID:     id,
			User:          user,
			Hostname:      hostname,
			AllowRemember: allowRemember,
		})

		select {
		case resp := <-ch:
			if !resp.ok {
				return "", false, nil
			}
			if allowRemember && resp.remember {
				if err := auth.setPassword(resp.password); err != nil {
					log.Printf("ssh-first: store password in secret service: %v", err)
				}
			}
			return resp.password, true, nil
		case <-time.After(promptTimeout):
			return "", false, fmt.Errorf("password prompt timed out")
		}
	}
}

// RespondPassword answers a pending "password:request" prompt. Set remember
// to persist the password in the platform secret store for future connections.
func (a *App) RespondPassword(requestID, password string, remember bool) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.password[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending password request %q", requestID)
	}
	ch <- passwordResponse{password: password, remember: remember, ok: true}
	return nil
}

// CancelPasswordPrompt lets the user dismiss a password dialog without
// submitting a value.
func (a *App) CancelPasswordPrompt(requestID string) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.password[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return nil
	}
	ch <- passwordResponse{ok: false}
	return nil
}

func (a *App) providePassphrase() sshpkg.PassphraseProvider {
	return func(identityFilePath string) (string, bool, error) {
		if pass, err := secrets.GetIdentityPassphrase(identityFilePath); err == nil {
			return pass, true, nil
		}

		id := uuid.NewString()
		ch := make(chan passphraseResponse, 1)
		a.pending.mu.Lock()
		a.pending.passphrase[id] = ch
		a.pending.mu.Unlock()
		defer func() {
			a.pending.mu.Lock()
			delete(a.pending.passphrase, id)
			a.pending.mu.Unlock()
		}()

		a.emit("passphrase:request", PassphrasePromptEvent{RequestID: id, IdentityFile: identityFilePath})

		select {
		case resp := <-ch:
			if !resp.ok {
				return "", false, nil
			}
			if resp.remember {
				if err := secrets.SetIdentityPassphrase(identityFilePath, resp.passphrase); err != nil {
					log.Printf("ssh-first: store passphrase in secret service: %v", err)
				}
			}
			return resp.passphrase, true, nil
		case <-time.After(promptTimeout):
			return "", false, fmt.Errorf("passphrase prompt timed out")
		}
	}
}

// RespondPassphrase answers a pending "passphrase:request" prompt.
func (a *App) RespondPassphrase(requestID, passphrase string, remember bool) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.passphrase[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending passphrase request %q", requestID)
	}
	ch <- passphraseResponse{passphrase: passphrase, remember: remember, ok: true}
	return nil
}

// CancelPassphrasePrompt lets the user dismiss a passphrase dialog without
// submitting a value.
func (a *App) CancelPassphrasePrompt(requestID string) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.passphrase[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return nil
	}
	ch <- passphraseResponse{ok: false}
	return nil
}

func (a *App) provideKeyboardInteractive() sshpkg.KeyboardInteractiveProvider {
	return func(user, hostname, instruction string, questions []string, echo []bool) ([]string, bool, error) {
		id := uuid.NewString()
		ch := make(chan keyboardInteractiveResponse, 1)
		a.pending.mu.Lock()
		a.pending.interactive[id] = ch
		a.pending.mu.Unlock()
		defer func() {
			a.pending.mu.Lock()
			delete(a.pending.interactive, id)
			a.pending.mu.Unlock()
		}()

		prompts := make([]KeyboardInteractiveQuestion, len(questions))
		for i, question := range questions {
			prompts[i] = KeyboardInteractiveQuestion{Prompt: question}
			if i < len(echo) {
				prompts[i].Echo = echo[i]
			}
		}
		a.emit("keyboard-interactive:request", KeyboardInteractivePromptEvent{
			RequestID:   id,
			User:        user,
			Hostname:    hostname,
			Instruction: instruction,
			Questions:   prompts,
		})

		select {
		case resp := <-ch:
			return resp.answers, resp.ok, nil
		case <-time.After(promptTimeout):
			return nil, false, fmt.Errorf("keyboard-interactive prompt timed out")
		}
	}
}

// RespondKeyboardInteractive answers a complete server challenge. Challenge
// responses may contain OTPs and are deliberately never stored.
func (a *App) RespondKeyboardInteractive(requestID string, answers []string) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.interactive[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending keyboard-interactive request %q", requestID)
	}
	ch <- keyboardInteractiveResponse{answers: append([]string(nil), answers...), ok: true}
	return nil
}

func (a *App) CancelKeyboardInteractivePrompt(requestID string) error {
	a.pending.mu.Lock()
	ch, ok := a.pending.interactive[requestID]
	a.pending.mu.Unlock()
	if !ok {
		return nil
	}
	ch <- keyboardInteractiveResponse{ok: false}
	return nil
}
