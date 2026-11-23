# Suporte

Este é um projeto mantido por uma pessoa. O objetivo desta página é deixar
claro **o que é suportado**, **onde perguntar** e **o que não é escopo**.

## Onde pedir ajuda

| Você precisa de… | Onde ir |
|------------------|---------|
| Instalar, configurar regras ou usar a UI | [Discussions · Q&A](https://github.com/in100tiva/whitelist-proxy/discussions/categories/q-a) |
| Reportar um bug reproduzível | [Issue de bug](.github/ISSUE_TEMPLATE/bug_report.yml) |
| Sugerir funcionalidade | [Issue de ideia](.github/ISSUE_TEMPLATE/feature_request.yml) |
| Reportar vulnerabilidade | [Advisory privado](https://github.com/in100tiva/whitelist-proxy/security/advisories/new) — **nunca** issue pública |
| Ler a documentação | [README](README.md) · [CHANGELOG](CHANGELOG.md) · [Limitações conhecidas](README.md#limitações-conhecidas) |

Antes de perguntar, uma busca rápida nas issues e discussions costuma resolver —
e o README responde à maioria das dúvidas de instalação e de sintaxe de regras.

## O que é suportado

- Windows 10/11 e Windows Server 2019/2022 (x64 e arm64).
- Compilação e execução do proxy em Linux/macOS **para desenvolvimento**
  (`go run ./cmd/proxy run`), sem integração com serviço ou registro do sistema.
- Releases pré-compiladas (6 alvos) publicadas na página de
  [Releases](https://github.com/in100tiva/whitelist-proxy/releases).
- As duas versões mais recentes de Chrome/Edge/Firefox que respeitem o proxy do
  sistema — lembrando que navegadores com DoH/QUIC ativos podem escapar
  (veja as limitações).

## O que não é suportado

- Ambientes corporativos com política de domínio obrigatória: o projeto ainda
  não aplica política via GPO/HKLM (está no roadmap de fase 2). Para esse caso,
  combine o proxy com regras de firewall de saída.
- Interceptação TLS/MITM — o projeto **não** faz isso por decisão de design
  (inspeção apenas por SNI).
- Distribuição de builds customizadas, integração com fornecedores ou
  desenvolvimento sob demanda. Para trabalho remunerado, fale com o mantenedor
  pelo perfil do GitHub.

## Antes de abrir issue

1. Confirme a versão: `proxy.exe version`.
2. Rode `proxy.exe status` e guarde a saída.
3. Verifique se o comportamento está em [Limitações conhecidas](README.md#limitações-conhecidas).
4. Remova domínios internos, tokens e dados de cliente dos logs antes de colar.

## Tempo de resposta

Projeto mantido em tempo parcial: issues são triadas em até **7 dias úteis**.
Falhas de segurança seguem os prazos da [política de segurança](SECURITY.md).
