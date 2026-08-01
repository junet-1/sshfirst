package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// PasswordProvider supplies an interactive password, typically backed by a
// native-looking frontend dialog. ok=false means the user cancelled.
type PasswordProvider func(user, hostname string) (password string, ok bool, err error)

// PassphraseProvider supplies the passphrase protecting an encrypted private
// key file. ok=false means the user cancelled.
type PassphraseProvider func(identityFilePath string) (passphrase string, ok bool, err error)

// KeyboardInteractiveProvider answers server-defined authentication
// challenges. A challenge may contain several prompts (for example password
// plus one-time code); echo is true only for non-secret answers.
type KeyboardInteractiveProvider func(user, hostname, instruction string, questions []string, echo []bool) (answers []string, ok bool, err error)

// ErrAuthCancelled is returned when the user declines a password/passphrase prompt.
var ErrAuthCancelled = errors.New("authentication cancelled by user")

// dialAgent connects to the running ssh-agent via SSH_AUTH_SOCK. It returns
// (nil, nil) if no agent is available rather than an error, since agent
// absence is a normal, non-fatal condition.
func dialAgent() (agent.ExtendedAgent, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil //nolint:nilerr // agent socket stale/unreachable is non-fatal, just skip it
	}
	return agent.NewClient(conn), nil
}

// buildAuthMethods assembles the ssh.AuthMethod list for a connection,
// preferring the SSH agent, then configured identity files, then an
// interactive password prompt — mirroring the OpenSSH client's own
// preference order.
func buildAuthMethods(identityFiles []string, wantPassword bool, user, hostname string, passphrase PassphraseProvider, password PasswordProvider, keyboardInteractive KeyboardInteractiveProvider, onAttempt func(string)) ([]xssh.AuthMethod, error) {
	var methods []xssh.AuthMethod
	markAttempt := func(method string) {
		if onAttempt != nil {
			onAttempt(method)
		}
	}

	if ag, err := dialAgent(); err == nil && ag != nil {
		methods = append(methods, xssh.PublicKeysCallback(func() ([]xssh.Signer, error) {
			markAttempt("agent")
			return ag.Signers()
		}))
	}

	if len(identityFiles) > 0 {
		signers, err := loadIdentitySigners(identityFiles, passphrase)
		if err != nil {
			return nil, err
		}
		if len(signers) > 0 {
			methods = append(methods, xssh.PublicKeysCallback(func() ([]xssh.Signer, error) {
				markAttempt("identity")
				return signers, nil
			}))
		}
	}

	// Password hosts deliberately skip public keys, but still offer
	// keyboard-interactive as a fallback because many servers expose password
	// plus OTP/2FA exclusively through that SSH method.
	var passwordMethod xssh.AuthMethod
	if password != nil {
		passwordMethod = xssh.PasswordCallback(func() (string, error) {
			markAttempt("password")
			pw, ok, err := password(user, hostname)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", ErrAuthCancelled
			}
			return pw, nil
		})
	}

	var interactiveMethod xssh.AuthMethod
	if keyboardInteractive != nil {
		interactiveMethod = xssh.KeyboardInteractive(keyboardInteractiveChallenge(hostname, keyboardInteractive, markAttempt))
	}

	if wantPassword {
		methods = methods[:0]
		if passwordMethod != nil {
			methods = append(methods, passwordMethod)
		}
		if interactiveMethod != nil {
			methods = append(methods, interactiveMethod)
		}
		return methods, nil
	}

	// For key-based profiles, OpenSSH-style keyboard-interactive comes before
	// the plain password fallback so challenge-based 2FA is not bypassed.
	if interactiveMethod != nil {
		methods = append(methods, interactiveMethod)
	}
	if passwordMethod != nil {
		methods = append(methods, passwordMethod)
	}

	return methods, nil
}

func keyboardInteractiveChallenge(hostname string, provider KeyboardInteractiveProvider, onAttempt func(string)) xssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echo []bool) ([]string, error) {
		if onAttempt != nil {
			onAttempt("keyboard-interactive")
		}
		answers, ok, err := provider(user, hostname, instruction, questions, echo)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrAuthCancelled
		}
		if len(answers) != len(questions) {
			return nil, fmt.Errorf("keyboard-interactive returned %d answers for %d questions", len(answers), len(questions))
		}
		return answers, nil
	}
}

func loadIdentitySigners(paths []string, passphrase PassphraseProvider) ([]xssh.Signer, error) {
	var signers []xssh.Signer
	for _, path := range paths {
		key, err := os.ReadFile(path)
		if err != nil {
			continue // an unreadable/missing identity file is skipped, not fatal — the agent or another key may still work
		}

		signer, err := xssh.ParsePrivateKey(key)
		if err == nil {
			signers = append(signers, signer)
			continue
		}

		var passphraseErr *xssh.PassphraseMissingError
		if !errors.As(err, &passphraseErr) {
			continue // not an encrypted-key error; skip this key rather than aborting the whole connection
		}
		if passphrase == nil {
			continue
		}
		pass, ok, ppErr := passphrase(path)
		if ppErr != nil {
			return nil, ppErr
		}
		if !ok {
			continue
		}
		signer, err = xssh.ParsePrivateKeyWithPassphrase(key, []byte(pass))
		if err != nil {
			return nil, fmt.Errorf("decrypt identity file %s: %w", path, err)
		}
		signers = append(signers, signer)
	}
	return signers, nil
}
