package storage

import "database/sql"

// ForwardKind mirrors internal/forwarding.Kind but lives here so the storage
// layer has no dependency on the engine. Values must stay in sync.
type ForwardKind string

const (
	ForwardLocal   ForwardKind = "local"
	ForwardRemote  ForwardKind = "remote"
	ForwardDynamic ForwardKind = "dynamic"
)

// ForwardRule is a persisted port-forwarding definition scoped to one host.
type ForwardRule struct {
	ID        int64       `json:"id"`
	HostID    int64       `json:"hostId"`
	Kind      ForwardKind `json:"kind"`
	Label     string      `json:"label"`
	BindAddr  string      `json:"bindAddr"`
	BindPort  int         `json:"bindPort"`
	DestHost  string      `json:"destHost"`
	DestPort  int         `json:"destPort"`
	AutoStart bool        `json:"autoStart"`
}

// ForwardRuleInput is the payload for creating/updating a rule.
type ForwardRuleInput struct {
	HostID    int64       `json:"hostId"`
	Kind      ForwardKind `json:"kind"`
	Label     string      `json:"label"`
	BindAddr  string      `json:"bindAddr"`
	BindPort  int         `json:"bindPort"`
	DestHost  string      `json:"destHost"`
	DestPort  int         `json:"destPort"`
	AutoStart bool        `json:"autoStart"`
}

// ListForwardRules returns every forwarding rule for a host, oldest first.
func (s *Store) ListForwardRules(hostID int64) ([]ForwardRule, error) {
	rows, err := s.db.Query(`
		SELECT id, host_id, kind, label, bind_addr, bind_port, dest_host, dest_port, auto_start
		FROM forward_rules
		WHERE host_id = ?
		ORDER BY id ASC`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []ForwardRule{}
	for rows.Next() {
		var r ForwardRule
		var kind string
		if err := rows.Scan(&r.ID, &r.HostID, &kind, &r.Label, &r.BindAddr, &r.BindPort, &r.DestHost, &r.DestPort, &r.AutoStart); err != nil {
			return nil, err
		}
		r.Kind = ForwardKind(kind)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetForwardRule loads a single rule by ID.
func (s *Store) GetForwardRule(id int64) (ForwardRule, error) {
	var r ForwardRule
	var kind string
	err := s.db.QueryRow(`
		SELECT id, host_id, kind, label, bind_addr, bind_port, dest_host, dest_port, auto_start
		FROM forward_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.HostID, &kind, &r.Label, &r.BindAddr, &r.BindPort, &r.DestHost, &r.DestPort, &r.AutoStart)
	if err == sql.ErrNoRows {
		return ForwardRule{}, ErrNotFound
	}
	if err != nil {
		return ForwardRule{}, err
	}
	r.Kind = ForwardKind(kind)
	return r, nil
}

// CreateForwardRule inserts a rule and returns it with its assigned ID.
func (s *Store) CreateForwardRule(input ForwardRuleInput) (ForwardRule, error) {
	res, err := s.db.Exec(`
		INSERT INTO forward_rules (host_id, kind, label, bind_addr, bind_port, dest_host, dest_port, auto_start)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.HostID, string(input.Kind), input.Label, input.BindAddr, input.BindPort, input.DestHost, input.DestPort, input.AutoStart)
	if err != nil {
		return ForwardRule{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ForwardRule{}, err
	}
	return s.GetForwardRule(id)
}

// UpdateForwardRule overwrites an existing rule's fields (host_id is not moved).
func (s *Store) UpdateForwardRule(id int64, input ForwardRuleInput) (ForwardRule, error) {
	res, err := s.db.Exec(`
		UPDATE forward_rules
		SET kind = ?, label = ?, bind_addr = ?, bind_port = ?, dest_host = ?, dest_port = ?, auto_start = ?
		WHERE id = ?`,
		string(input.Kind), input.Label, input.BindAddr, input.BindPort, input.DestHost, input.DestPort, input.AutoStart, id)
	if err != nil {
		return ForwardRule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ForwardRule{}, ErrNotFound
	}
	return s.GetForwardRule(id)
}

// DeleteForwardRule removes a rule.
func (s *Store) DeleteForwardRule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM forward_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
