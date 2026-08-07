package main

import "testing"

func TestShouldStartHidden(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "ordinary launch", args: nil, want: false},
		{name: "tray autostart", args: []string{"--start-in-tray"}, want: true},
		{name: "hidden alias", args: []string{"--hidden"}, want: true},
		{name: "unrelated arguments", args: []string{"--debug"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldStartHidden(test.args); got != test.want {
				t.Fatalf("shouldStartHidden(%v) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}
