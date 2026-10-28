package inner

import "testing"

func TestAdd(t *testing.T) {
	s := add(1, 2)

	if s != 3 {
		t.Errorf("add(1, 2) = %d; want 3", s)
	}
}
