package filter

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Rule representa uma única regra da whitelist/blacklist.
type Rule struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"`             // "exact", "wildcard" ou "regex"
	Action  string `json:"action,omitempty"` // "allow" (default) | "block"
	Note    string `json:"note,omitempty"`
}

// Matcher é seguro para uso concorrente. Load() troca o conjunto de regras
// atomicamente. Regras "block" têm prioridade sobre regras "allow".
type Matcher struct {
	mu            sync.RWMutex
	exact         map[string]bool
	wildcard      []string
	regex         []*regexp.Regexp
	blockExact    map[string]bool
	blockWildcard []string
	blockRegex    []*regexp.Regexp
	rules         []Rule
	defaultAllow  bool // true = blacklist (permite tudo por padrão), false = whitelist
}

// New devolve um matcher vazio em modo whitelist (bloqueia tudo por padrão).
func New() *Matcher {
	return &Matcher{
		exact:      make(map[string]bool),
		blockExact: make(map[string]bool),
	}
}

// SetMode define o comportamento padrão quando nenhuma regra bate:
// "blacklist" → permite tudo por padrão; qualquer outro valor → whitelist.
func (m *Matcher) SetMode(mode string) {
	m.mu.Lock()
	m.defaultAllow = mode == "blacklist"
	m.mu.Unlock()
}

// Mode devolve o modo atual ("blacklist" ou "whitelist").
func (m *Matcher) Mode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultAllow {
		return "blacklist"
	}
	return "whitelist"
}

// stripSchemeAndPath remove esquema (https://) e caminho (/foo) de um padrão,
// deixando apenas o host. Torna o matcher tolerante a entradas como
// "https://exemplo.com" que deveriam ser só "exemplo.com".
func stripSchemeAndPath(s string) string {
	if i := strings.Index(s, "://"); i != -1 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i != -1 {
		s = s[:i]
	}
	return s
}

// Load substitui o conjunto atual de regras atomicamente. Em caso de erro
// de validação o matcher mantém o estado anterior intocado.
func (m *Matcher) Load(rules []Rule) error {
	exact := make(map[string]bool)
	var wildcard []string
	var regs []*regexp.Regexp
	blockExact := make(map[string]bool)
	var blockWildcard []string
	var blockRegs []*regexp.Regexp

	for _, r := range rules {
		isBlock := r.Action == "block"

		switch r.Type {
		case "exact", "":
			p := strings.ToLower(strings.TrimSpace(stripSchemeAndPath(r.Pattern)))
			if p == "" {
				return fmt.Errorf("padrão exact vazio")
			}
			if isBlock {
				blockExact[p] = true
			} else {
				exact[p] = true
			}

		case "wildcard":
			p := strings.ToLower(strings.TrimSpace(r.Pattern))
			if !strings.HasPrefix(p, "*.") || len(p) < 3 {
				return fmt.Errorf("wildcard inválido %q (formato esperado: *.exemplo.com)", r.Pattern)
			}
			suffix := p[1:]
			root := p[2:]
			if isBlock {
				blockWildcard = append(blockWildcard, suffix)
				blockExact[root] = true
			} else {
				wildcard = append(wildcard, suffix)
				exact[root] = true
			}

		case "regex":
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return fmt.Errorf("regex inválida %q: %w", r.Pattern, err)
			}
			if isBlock {
				blockRegs = append(blockRegs, re)
			} else {
				regs = append(regs, re)
			}

		default:
			return fmt.Errorf("tipo de regra desconhecido %q", r.Type)
		}
	}

	m.mu.Lock()
	m.exact, m.wildcard, m.regex = exact, wildcard, regs
	m.blockExact, m.blockWildcard, m.blockRegex = blockExact, blockWildcard, blockRegs
	m.rules = rules
	m.mu.Unlock()
	return nil
}

// Allowed devolve true se o host é permitido. Regras "block" têm prioridade
// sobre regras "allow". O host pode vir com porta (ex.: "exemplo.com:443").
func (m *Matcher) Allowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i != -1 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Regras de bloqueio têm prioridade.
	if m.blockExact[host] {
		return false
	}
	for _, suffix := range m.blockWildcard {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	for _, re := range m.blockRegex {
		if re.MatchString(host) {
			return false
		}
	}

	// Regras de permissão.
	if m.exact[host] {
		return true
	}
	for _, suffix := range m.wildcard {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	for _, re := range m.regex {
		if re.MatchString(host) {
			return true
		}
	}
	return m.defaultAllow
}

// Rules devolve uma cópia das regras carregadas (para o endpoint admin).
func (m *Matcher) Rules() []Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Rule, len(m.rules))
	copy(out, m.rules)
	return out
}
