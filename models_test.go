package main

import "testing"

func TestIsValidTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusOpen, StatusInProgress, true},
		{StatusInProgress, StatusClosed, true},
		{StatusOpen, StatusClosed, false},     // no skipping
		{StatusInProgress, StatusOpen, false}, // no going backward
		{StatusClosed, StatusOpen, false},     // closed never reopens
		{StatusClosed, StatusInProgress, false},
		{StatusOpen, StatusOpen, false}, // no-op isn't a transition
	}

	for _, c := range cases {
		got := isValidTransition(c.from, c.to)
		if got != c.want {
			t.Errorf("isValidTransition(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
