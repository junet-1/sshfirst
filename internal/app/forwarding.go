package app

import (
	"fmt"
	"log"

	"ssh-first/internal/forwarding"
	"ssh-first/internal/storage"
)

// activeForward pairs a running tunnel with the rule that created it, so the
// frontend can be told what a connection is currently forwarding.
type activeForward struct {
	tunnel *forwarding.Tunnel
	rule   storage.ForwardRule
}

// ActiveForward describes a running forward for the frontend.
type ActiveForward struct {
	RuleID    int64  `json:"ruleId"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	BoundAddr string `json:"boundAddr"`
}

// ForwardStatusEvent is emitted on "forward:status" when a forward starts or
// stops, and on "forward:error" (Error set) for a non-fatal per-connection
// problem within a running forward.
type ForwardStatusEvent struct {
	ConnectionID string `json:"connectionId"`
	RuleID       int64  `json:"ruleId"`
	Active       bool   `json:"active"`
	Kind         string `json:"kind,omitempty"`
	BoundAddr    string `json:"boundAddr,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ListForwardRules returns the saved forwarding rules for a host.
func (a *App) ListForwardRules(hostID int64) ([]storage.ForwardRule, error) {
	if err := a.requireStore(); err != nil {
		return nil, err
	}
	return a.store.ListForwardRules(hostID)
}

// CreateForwardRule saves a new forwarding rule for a host.
func (a *App) CreateForwardRule(input storage.ForwardRuleInput) (storage.ForwardRule, error) {
	if err := a.requireStore(); err != nil {
		return storage.ForwardRule{}, err
	}
	if err := specFromRuleInput(input).Validate(); err != nil {
		return storage.ForwardRule{}, err
	}
	return a.store.CreateForwardRule(input)
}

// UpdateForwardRule overwrites a rule. A running instance of the rule is left
// untouched; the change takes effect the next time it is started.
func (a *App) UpdateForwardRule(id int64, input storage.ForwardRuleInput) (storage.ForwardRule, error) {
	if err := a.requireStore(); err != nil {
		return storage.ForwardRule{}, err
	}
	if err := specFromRuleInput(input).Validate(); err != nil {
		return storage.ForwardRule{}, err
	}
	return a.store.UpdateForwardRule(id, input)
}

// DeleteForwardRule removes a rule and stops it wherever it is currently
// running.
func (a *App) DeleteForwardRule(id int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	a.stopForwardEverywhere(id)
	return a.store.DeleteForwardRule(id)
}

// StartForward opens the tunnel for ruleID on an established connection.
func (a *App) StartForward(connectionID string, ruleID int64) (ActiveForward, error) {
	if err := a.requireStore(); err != nil {
		return ActiveForward{}, err
	}
	rule, err := a.store.GetForwardRule(ruleID)
	if err != nil {
		return ActiveForward{}, err
	}

	a.mu.Lock()
	cs, ok := a.connections[connectionID]
	if !ok {
		a.mu.Unlock()
		return ActiveForward{}, fmt.Errorf("no such connection %q", connectionID)
	}
	if cs.status != StatusConnected || cs.client == nil {
		a.mu.Unlock()
		return ActiveForward{}, fmt.Errorf("connection %q is not ready", connectionID)
	}
	if existing, running := cs.forwards[ruleID]; running {
		af := activeForwardInfo(existing)
		a.mu.Unlock()
		return af, nil
	}
	client := cs.client
	a.mu.Unlock()

	tunnel, err := forwarding.Open(client.Client, specFromRule(rule), a.forwardErrorReporter(connectionID, rule))
	if err != nil {
		return ActiveForward{}, err
	}

	a.mu.Lock()
	// Re-check under the lock: the connection may have gone away, or another
	// call may have won the race to start this same rule, while Open ran.
	cs, ok = a.connections[connectionID]
	if !ok || cs.status != StatusConnected {
		a.mu.Unlock()
		_ = tunnel.Close()
		return ActiveForward{}, fmt.Errorf("connection %q is no longer ready", connectionID)
	}
	if existing, running := cs.forwards[ruleID]; running {
		a.mu.Unlock()
		_ = tunnel.Close()
		return activeForwardInfo(existing), nil
	}
	af := &activeForward{tunnel: tunnel, rule: rule}
	cs.forwards[ruleID] = af
	a.mu.Unlock()

	info := activeForwardInfo(af)
	a.emit("forward:status", ForwardStatusEvent{
		ConnectionID: connectionID,
		RuleID:       ruleID,
		Active:       true,
		Kind:         info.Kind,
		BoundAddr:    info.BoundAddr,
	})
	return info, nil
}

// StopForward closes a running forward without touching its saved rule.
func (a *App) StopForward(connectionID string, ruleID int64) error {
	a.mu.Lock()
	cs, ok := a.connections[connectionID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("no such connection %q", connectionID)
	}
	af, running := cs.forwards[ruleID]
	if running {
		delete(cs.forwards, ruleID)
	}
	a.mu.Unlock()
	if !running {
		return nil
	}
	err := af.tunnel.Close()
	a.emit("forward:status", ForwardStatusEvent{ConnectionID: connectionID, RuleID: ruleID, Active: false})
	return err
}

// ListActiveForwards reports which of a connection's forwards are running.
func (a *App) ListActiveForwards(connectionID string) ([]ActiveForward, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cs, ok := a.connections[connectionID]
	if !ok {
		return nil, fmt.Errorf("no such connection %q", connectionID)
	}
	out := make([]ActiveForward, 0, len(cs.forwards))
	for _, af := range cs.forwards {
		out = append(out, activeForwardInfo(af))
	}
	return out, nil
}

// autoStartForwards opens every rule flagged auto_start once a host connects.
// Failures are reported but never block the connection.
func (a *App) autoStartForwards(connectionID string, host storage.Host) {
	if host.ID <= 0 || a.store == nil {
		return
	}
	rules, err := a.store.ListForwardRules(host.ID)
	if err != nil {
		log.Printf("ssh-first: list forward rules for auto-start: %v", err)
		return
	}
	for _, rule := range rules {
		if !rule.AutoStart {
			continue
		}
		if _, err := a.StartForward(connectionID, rule.ID); err != nil {
			log.Printf("ssh-first: auto-start forward %d: %v", rule.ID, err)
			a.emit("forward:error", ForwardStatusEvent{
				ConnectionID: connectionID,
				RuleID:       rule.ID,
				Active:       false,
				Error:        safeErrorMessage(err),
			})
		}
	}
}

// restartForwards reopens forwards that were active before a reconnect. Best
// effort: a rule that has since been deleted or now fails to bind is skipped.
func (a *App) restartForwards(connectionID string, ruleIDs []int64) {
	for _, id := range ruleIDs {
		if _, err := a.StartForward(connectionID, id); err != nil {
			log.Printf("ssh-first: reopen forward %d after reconnect: %v", id, err)
			a.emit("forward:error", ForwardStatusEvent{
				ConnectionID: connectionID,
				RuleID:       id,
				Active:       false,
				Error:        safeErrorMessage(err),
			})
		}
	}
}

// stopForwardEverywhere closes ruleID on every connection currently running it
// (a rule is per-host, and a host may have several open connections).
func (a *App) stopForwardEverywhere(ruleID int64) {
	type target struct {
		connectionID string
		af           *activeForward
	}
	var targets []target
	a.mu.Lock()
	for id, cs := range a.connections {
		if af, running := cs.forwards[ruleID]; running {
			delete(cs.forwards, ruleID)
			targets = append(targets, target{connectionID: id, af: af})
		}
	}
	a.mu.Unlock()
	for _, t := range targets {
		_ = t.af.tunnel.Close()
		a.emit("forward:status", ForwardStatusEvent{ConnectionID: t.connectionID, RuleID: ruleID, Active: false})
	}
}

// forwardErrorReporter surfaces a running forward's non-fatal per-connection
// errors (a failed dial, a bad SOCKS request) to the frontend and the log.
func (a *App) forwardErrorReporter(connectionID string, rule storage.ForwardRule) func(error) {
	return func(err error) {
		if err == nil {
			return
		}
		log.Printf("ssh-first: forward %d (%s): %v", rule.ID, rule.Kind, err)
		a.emit("forward:error", ForwardStatusEvent{
			ConnectionID: connectionID,
			RuleID:       rule.ID,
			Active:       true,
			Kind:         string(rule.Kind),
			Error:        safeErrorMessage(err),
		})
	}
}

func activeForwardInfo(af *activeForward) ActiveForward {
	return ActiveForward{
		RuleID:    af.rule.ID,
		Kind:      string(af.rule.Kind),
		Label:     af.rule.Label,
		BoundAddr: af.tunnel.Addr(),
	}
}

func specFromRule(r storage.ForwardRule) forwarding.Spec {
	return forwarding.Spec{
		Kind:     forwarding.Kind(r.Kind),
		BindAddr: r.BindAddr,
		BindPort: r.BindPort,
		DestHost: r.DestHost,
		DestPort: r.DestPort,
	}
}

func specFromRuleInput(in storage.ForwardRuleInput) forwarding.Spec {
	return forwarding.Spec{
		Kind:     forwarding.Kind(in.Kind),
		BindAddr: in.BindAddr,
		BindPort: in.BindPort,
		DestHost: in.DestHost,
		DestPort: in.DestPort,
	}
}
