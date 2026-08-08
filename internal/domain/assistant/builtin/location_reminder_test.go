package builtin

import "testing"

func TestNormalizePlaceKeyword(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"any nearby Alfamart", "Alfamart"},
		{"nearby Alfamart", "Alfamart"},
		{"Alfamart", "Alfamart"},
		{"  any  nearby  Indomaret  ", "Indomaret"},
	}
	for _, tc := range tests {
		if got := NormalizePlaceKeyword(tc.in); got != tc.want {
			t.Errorf("NormalizePlaceKeyword(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsVaguePlaceQuery(t *testing.T) {
	vague := []string{"home", "my home", "work", "office"}
	for _, q := range vague {
		if !IsVaguePlaceQuery(q) {
			t.Errorf("expected %q to be vague", q)
		}
	}
	if IsVaguePlaceQuery("Cempaka Putih Tengah 20") {
		t.Fatal("expected full address not to be vague")
	}
}
