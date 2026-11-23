package filter

import (
	"strings"
	"testing"
)

// seedMatcher devolve um matcher com uma política fixa e representativa
// (exact, wildcard e regex, tanto de allow quanto de block), usada pelos
// alvos de fuzzing abaixo.
func seedMatcher(tb testing.TB) *Matcher {
	tb.Helper()
	m := New()
	rules := []Rule{
		{Pattern: "exemplo.com", Type: "exact"},
		{Pattern: "https://site.com/pagina", Type: "exact"},
		{Pattern: "*.permitido.com", Type: "wildcard"},
		{Pattern: `^app[0-9]+\.interno\.test$`, Type: "regex"},
		{Pattern: "bloqueado.com", Type: "exact", Action: "block"},
		{Pattern: "*.malicioso.com", Type: "wildcard", Action: "block"},
		{Pattern: `^tracker[0-9]*\.ads\.`, Type: "regex", Action: "block"},
	}
	if err := m.Load(rules); err != nil {
		tb.Fatalf("Load: %v", err)
	}
	return m
}

// FuzzAllowed exercita a decisão allow/deny com hosts arbitrários. O host vem
// da rede (header Host do HTTP, SNI do HTTPS), então a decisão precisa ser
// determinística, não pode entrar em pânico e as regras de bloqueio precisam
// manter prioridade sobre as de liberação.
func FuzzAllowed(f *testing.F) {
	seeds := []string{
		"exemplo.com", "https://exemplo.com/rota", "EXEMPLO.COM",
		"site.com", "site.com/qualquer/coisa",
		"api.permitido.com", "permitido.com",
		"app42.interno.test", "app.interno.test",
		"bloqueado.com", "x.malicioso.com", "malicioso.com",
		"tracker99.ads.net", "tracker.ads.net",
		"exemplo.com:443", "exemplo.com.", "exemplo.com..", "",
		":", ":", "[::1]:8080", "exemplo.com\x00.evil",
		strings.Repeat("a.", 200),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	m := seedMatcher(f)

	f.Fuzz(func(t *testing.T, host string) {
		first := m.Allowed(host)
		if second := m.Allowed(host); second != first {
			t.Fatalf("decisão não determinística para %q: %v != %v", host, first, second)
		}

		// Para as formas canônicas exatas, a regra de bloqueio precisa vencer.
		canon := strings.ToLower(strings.TrimSpace(host))
		if i := strings.LastIndex(canon, ":"); i != -1 && !strings.Contains(canon[i:], "]") {
			canon = canon[:i]
		}
		canon = strings.TrimSuffix(canon, ".")
		if canon == "bloqueado.com" && first {
			t.Fatalf("host de bloqueio liberado: %q", host)
		}
	})
}

// FuzzLoad alimenta o carregador de regras com combinações arbitrárias de
// padrão, tipo e ação. Invariantes: entrada inválida nunca altera o estado
// atual (carga é atômica) e entrada aceita nunca derruba Allowed.
func FuzzLoad(f *testing.F) {
	f.Add("exemplo.com", "exact", "allow")
	f.Add("https://exemplo.com/x", "", "")
	f.Add("*.exemplo.com", "wildcard", "block")
	f.Add(`^a+$`, "regex", "")
	f.Add("", "", "")
	f.Add("(", "regex", "allow")
	f.Add("*.", "wildcard", "")
	f.Add("x", "tipo-inexistente", "")

	f.Fuzz(func(t *testing.T, pattern, ruleType, action string) {
		m := New()
		err := m.Load([]Rule{{Pattern: pattern, Type: ruleType, Action: action}})
		if err != nil {
			if got := len(m.Rules()); got != 0 {
				t.Fatalf("Load com erro deixou estado: %d regras", got)
			}
			return
		}
		if got := len(m.Rules()); got != 1 {
			t.Fatalf("Load sem erro deveria ter 1 regra, tem %d", got)
		}
		_ = m.Allowed(pattern) // padrão aceito não pode entrar em pânico
	})
}
