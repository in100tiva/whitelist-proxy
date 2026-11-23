# Changelog

Todas as mudanças relevantes deste projeto são registradas aqui.

O formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o
projeto adere ao [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [Não publicado]

### Adicionado

- Builds pré-compiladas para Windows, Linux e macOS (amd64 e arm64) publicadas
  automaticamente em cada tag, com checksums SHA-256 e atestação de procedência.
- Subcomando `version` (`proxy.exe version`), permitindo identificar a build em
  execução — útil em relatos de bug.
- Pipeline de CI (Linux + Windows): `gofmt`, `go vet`, `go build`, testes com
  `-race` e compilação cruzada para os 6 alvos.
- Análise estática com CodeQL (Go e JavaScript) e OpenSSF Scorecard.
- Dependabot semanal para módulos Go e para as próprias GitHub Actions.
- Arquivos de comunidade: `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SUPPORT.md`, templates de issue e pull request, `CODEOWNERS` e `CITATION.cff`.
- Testes de fuzzing nativos do Go — `FuzzAllowed` e `FuzzLoad` (matcher de
  regras) e `FuzzParseSNI` (parser de `ClientHello`) — mais o workflow
  `Fuzzing`, que roda cada alvo por 90 s toda semana e guarda o corpus quando
  encontra uma entrada que quebra o parser.

### Alterado

- Módulo Go renomeado de `github.com/in100tiva/goproxy/whitelist-proxy` para
  `github.com/in100tiva/whitelist-proxy`, permitindo
  `go install github.com/in100tiva/whitelist-proxy/cmd/proxy@latest`.
- Formatação `gofmt` de `internal/logger/logger.go` e `internal/sysproxy/other.go`.
- README reescrito: badges vivos de CI/CodeQL/release, diagrama de fluxo,
  caminho de instalação a partir das releases e link do roadmap para as issues.
- Dependências atualizadas: `golang.org/x/sys` v0.50.0 e as GitHub Actions usadas
  nos workflows. Com isso, o projeto passa a exigir Go 1.26+.
- Todas as GitHub Actions passaram a ser referenciadas pelo SHA do commit
  (com a versão em comentário, ex.: `# v7`) para que o Dependabot continue
  atualizando sem que o código executado na CI possa mudar por baixo.

## [1.0.0] - 2026-11-23

Primeira versão estável.

### Adicionado

- Proxy local com política de **whitelist** (e modo blacklist) por domínio,
  rodando como Windows Service via `golang.org/x/sys/windows/svc`.
- Configuração automática do proxy do sistema no registro do Windows
  (`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`), revertida
  ao parar o serviço.
- Filtragem de HTTPS pelo **SNI** do `ClientHello`, sem MITM e sem certificado
  raiz instalado nos clientes; filtragem de HTTP pelo header `Host`.
- Regras `exact`, `wildcard` (`*.exemplo.com`, inclui o domínio raiz) e `regex`.
- Hot-reload do `whitelist.json` por polling de `mtime`, com validação atômica
  (arquivo inválido não derruba a política em vigor).
- Interface web embutida (`embed.FS`) com abas Whitelist, Logs e Status, incluindo
  testador de host.
- API administrativa em `127.0.0.1:8081` com autenticação por Bearer token gerado
  em `admin.token`, e caminhos legados sem prefixo `/api` mantidos por
  compatibilidade.
- Logs estruturados em JSONL com rotação diária e buffer circular em memória.
- Script `install.ps1` para copiar binário e whitelist, registrar e iniciar o serviço.
- Configuração automática de navegadores.
- Testes para o matcher de regras e para o parser de SNI (ClientHello TLS real
  gerado por `crypto/tls` contra loopback).

[1.0.0]: https://github.com/in100tiva/whitelist-proxy/releases/tag/v1.0.0
