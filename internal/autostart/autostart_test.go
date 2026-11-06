package autostart

import "testing"

func TestReconcileFollowsTheSystem(t *testing.T) {
	cases := []struct {
		name    string
		stored  bool
		state   State
		want    bool
		changed bool
	}{
		{"system settings removed it", true, Disabled, false, true},
		{"registered out of band", false, Enabled, true, true},
		{"already in sync", true, Enabled, true, false},
		{"already off", false, Disabled, false, false},
		// Unknown is a dev build or an unsupported platform: the stored choice
		// has to survive, or the flag flips every launch.
		{"silent about unknown on", true, Unknown, true, false},
		{"silent about unknown off", false, Unknown, false, false},
	}

	for _, c := range cases {
		next, changed := Reconcile(c.stored, c.state)
		if next != c.want || changed != c.changed {
			t.Errorf("%s: Reconcile(%v, %v) = (%v, %v), want (%v, %v)",
				c.name, c.stored, c.state, next, changed, c.want, c.changed)
		}
	}
}
