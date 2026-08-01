package ssh

import (
	"errors"
	"reflect"
	"testing"
)

func TestKeyboardInteractiveChallengeForwardsAllQuestions(t *testing.T) {
	var attempted string
	challenge := keyboardInteractiveChallenge(
		"server.example",
		func(user, hostname, instruction string, questions []string, echo []bool) ([]string, bool, error) {
			if user != "julian" || hostname != "server.example" || instruction != "Two-factor login" {
				t.Fatalf("unexpected challenge metadata: %q %q %q", user, hostname, instruction)
			}
			if !reflect.DeepEqual(questions, []string{"Password: ", "OTP: "}) {
				t.Fatalf("questions = %#v", questions)
			}
			if !reflect.DeepEqual(echo, []bool{false, false}) {
				t.Fatalf("echo = %#v", echo)
			}
			return []string{"secret", "123456"}, true, nil
		},
		func(method string) { attempted = method },
	)

	answers, err := challenge("julian", "Two-factor login", []string{"Password: ", "OTP: "}, []bool{false, false})
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if !reflect.DeepEqual(answers, []string{"secret", "123456"}) {
		t.Fatalf("answers = %#v", answers)
	}
	if attempted != "keyboard-interactive" {
		t.Fatalf("attempted method = %q", attempted)
	}
}

func TestKeyboardInteractiveChallengeCancellation(t *testing.T) {
	challenge := keyboardInteractiveChallenge(
		"server.example",
		func(string, string, string, []string, []bool) ([]string, bool, error) {
			return nil, false, nil
		},
		nil,
	)

	_, err := challenge("julian", "", []string{"OTP: "}, []bool{false})
	if !errors.Is(err, ErrAuthCancelled) {
		t.Fatalf("error = %v, want ErrAuthCancelled", err)
	}
}

func TestKeyboardInteractiveChallengeRejectsWrongAnswerCount(t *testing.T) {
	challenge := keyboardInteractiveChallenge(
		"server.example",
		func(string, string, string, []string, []bool) ([]string, bool, error) {
			return []string{"only one"}, true, nil
		},
		nil,
	)

	if _, err := challenge("julian", "", []string{"One", "Two"}, []bool{false, false}); err == nil {
		t.Fatal("expected answer-count error")
	}
}
