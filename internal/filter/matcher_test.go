package filter

import "testing"

func TestExactMatch(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "google.com", Type: "exact"}}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host string
		want bool
	}{
		{"google.com", true},
		{"GOOGLE.COM", true},
		{"google.com:443", true},
		{"google.com.", true},
		{"www.google.com", false},
		{"notgoogle.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := m.Allowed(c.host); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestWildcardMatch(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "*.google.com", Type: "wildcard"}}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host string
		want bool
	}{
		{"www.google.com", true},
		{"a.b.c.google.com", true},
		{"google.com", true},     // o domínio raiz também é liberado
		{"notgoogle.com", false}, // não pode dar falso positivo
		{"google.com.evil.com", false},
		{"fakegoogle.com", false},
	}
	for _, c := range cases {
		if got := m.Allowed(c.host); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestRegexMatch(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: `^([a-z0-9-]+\.)?empresa\.com\.br$`, Type: "regex"}}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host string
		want bool
	}{
		{"empresa.com.br", true},
		{"app.empresa.com.br", true},
		{"a-b.empresa.com.br", true},
		{"x.y.empresa.com.br", false},
		{"empresa.com", false},
	}
	for _, c := range cases {
		if got := m.Allowed(c.host); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestInvalidWildcard(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "google.com", Type: "wildcard"}}); err == nil {
		t.Fatal("esperava erro para wildcard sem prefixo *.")
	}
}

func TestInvalidRegex(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "[invalid", Type: "regex"}}); err == nil {
		t.Fatal("esperava erro para regex inválida")
	}
}

func TestUnknownType(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "x", Type: "fuzzy"}}); err == nil {
		t.Fatal("esperava erro para tipo desconhecido")
	}
}

func TestEmptyMatcherBlocksEverything(t *testing.T) {
	m := New()
	if m.Allowed("google.com") {
		t.Fatal("matcher vazio deveria bloquear tudo")
	}
}

func TestLoadAtomicityOnError(t *testing.T) {
	m := New()
	if err := m.Load([]Rule{{Pattern: "google.com", Type: "exact"}}); err != nil {
		t.Fatal(err)
	}
	// Tenta carregar regras inválidas — o estado anterior precisa permanecer.
	if err := m.Load([]Rule{{Pattern: "[invalid", Type: "regex"}}); err == nil {
		t.Fatal("esperava erro de validação")
	}
	if !m.Allowed("google.com") {
		t.Fatal("Load com erro não deveria ter sobrescrito o estado anterior")
	}
}
