package domain

import "testing"

func TestFirstName(t *testing.T) {
	for in, want := range map[string]string{"Сидоров Пётр Андреевич": "Пётр", "Анна Смирнова": "Анна", "Олег": "Олег", " ": "коллега"} {
		if got := FirstName(in); got != want {
			t.Errorf("FirstName(%q) = %q, want %q", in, got, want)
		}
	}
}
