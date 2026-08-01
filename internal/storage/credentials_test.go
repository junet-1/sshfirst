package storage

import "testing"

func TestCredentialCRUDAndHostReference(t *testing.T) {
	s := newTestStore(t)

	cred, err := s.CreateCredential(CredentialInput{
		Name:          "deploy",
		User:          "deploy",
		AuthMethod:    AuthMethodIdentity,
		IdentityFiles: []string{"~/.ssh/id_ed25519"},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if cred.ID == 0 || cred.User != "deploy" || len(cred.IdentityFiles) != 1 {
		t.Fatalf("unexpected credential: %+v", cred)
	}

	// A host can reference the credential.
	host, err := s.CreateHost(HostInput{Label: "web1", Hostname: "web1.example.com", CredentialID: &cred.ID}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if host.CredentialID == nil || *host.CredentialID != cred.ID {
		t.Fatalf("credential reference did not round-trip: %+v", host.CredentialID)
	}

	// Updating the credential is reflected on lookup.
	if _, err := s.UpdateCredential(cred.ID, CredentialInput{Name: "deploy-renamed", User: "root", AuthMethod: AuthMethodAgent}); err != nil {
		t.Fatalf("UpdateCredential: %v", err)
	}
	updated, err := s.GetCredential(cred.ID)
	if err != nil || updated.Name != "deploy-renamed" || updated.User != "root" || updated.AuthMethod != AuthMethodAgent {
		t.Fatalf("unexpected updated credential: %+v (err %v)", updated, err)
	}

	// Deleting the credential reverts the host to inline (credential_id NULL)
	// rather than deleting the host — the ON DELETE SET NULL foreign key.
	if err := s.DeleteCredential(cred.ID); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	reverted, err := s.GetHost(host.ID)
	if err != nil {
		t.Fatalf("GetHost after credential delete: %v", err)
	}
	if reverted.CredentialID != nil {
		t.Fatalf("host should have reverted to inline auth, got credentialID %v", *reverted.CredentialID)
	}
}
